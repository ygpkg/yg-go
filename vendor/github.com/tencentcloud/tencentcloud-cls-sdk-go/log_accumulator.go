package tencentcloud_cls_sdk_go

import (
	"errors"
	"sync"
	asyncAtomic "sync/atomic"

	"go.uber.org/atomic"
)

type Accumulator struct {
	lock           sync.RWMutex
	logTopicData   map[string]*ProducerBatch
	producerConfig *AsyncProducerClientConfig
	worker         *Worker
	shutDownFlag   *atomic.Bool
	threadPool     *SendThreadPool
	producer       *AsyncProducerClient
	batchID        *atomic.Int64
	producerHash   string
}

// NewAccumulator ...
func NewAccumulator(config *AsyncProducerClientConfig, worker *Worker, threadPool *SendThreadPool, producer *AsyncProducerClient) *Accumulator {
	return &Accumulator{
		logTopicData:   make(map[string]*ProducerBatch),
		producerConfig: config,
		worker:         worker,
		shutDownFlag:   atomic.NewBool(false),
		threadPool:     threadPool,
		producer:       producer,
		batchID:        atomic.NewInt64(0),
		producerHash:   producer.producerHash,
	}
}

func (accumulator *Accumulator) addOrSendProducerBatch(topicId string, producerBatch *ProducerBatch, log interface{}, callback CallBack, logSize int64) {
	totalDataCount := producerBatch.getLogGroupCount() + 1
	if producerBatch.totalDataSize > accumulator.producerConfig.MaxBatchSize &&
		producerBatch.totalDataSize < 5242880 &&
		totalDataCount <= accumulator.producerConfig.MaxBatchCount {
		producerBatch.addLogToLogGroup(log)
		if callback != nil {
			producerBatch.addProducerBatchCallBack(callback)
		}
		accumulator.innerSendToServer(topicId, producerBatch)
	} else if producerBatch.totalDataSize <= accumulator.producerConfig.MaxBatchSize &&
		totalDataCount <= accumulator.producerConfig.MaxBatchCount {
		producerBatch.addLogToLogGroup(log)
		if callback != nil {
			producerBatch.addProducerBatchCallBack(callback)
		}
	} else {
		// yg-go patch: the caller charged this log to the outgoing batch, but it
		// goes into the new batch instead. Take the charge back so each batch is
		// accounted for the logs it actually carries. Keep while vendoring.
		asyncAtomic.AddInt64(&producerBatch.totalDataSize, -logSize)
		asyncAtomic.AddInt64(&accumulator.producer.producerLogGroupSize, -logSize)
		accumulator.innerSendToServer(topicId, producerBatch)
		accumulator.createNewProducerBatch(log, callback, topicId)
	}
}

func (accumulator *Accumulator) createNewProducerBatch(logType interface{}, callback CallBack, topicId string) {
	if item, ok := logType.(*Log); ok {
		newProducerBatch := NewProducerBatch(topicId, accumulator.producerConfig, callback, item, generatePackageId(accumulator.producerHash, accumulator.batchID))
		accumulator.trackNewBatch(topicId, newProducerBatch)
	} else if logList, ok := logType.([]*Log); ok {
		newProducerBatch := NewProducerBatch(topicId, accumulator.producerConfig, callback, logList, generatePackageId(accumulator.producerHash, accumulator.batchID))
		accumulator.trackNewBatch(topicId, newProducerBatch)
	}
}

// yg-go patch: addLogToProducerBatch only charges logs appended to an existing
// batch, while the worker deducts the whole batch when it finishes. Without
// charging the first log as well, producerLogGroupSize drifted negative and the
// MaxBlockSec backpressure stopped triggering. Keep while vendoring.
func (accumulator *Accumulator) trackNewBatch(topicId string, batch *ProducerBatch) {
	accumulator.logTopicData[topicId] = batch
	asyncAtomic.AddInt64(&accumulator.producer.producerLogGroupSize, batch.totalDataSize)
	asyncAtomic.AddInt64(&accumulator.producer.outstandingBatches, 1)
}

func (accumulator *Accumulator) innerSendToServer(topicId string, producerBatch *ProducerBatch) {
	accumulator.threadPool.addTask(producerBatch)
	delete(accumulator.logTopicData, topicId)
}

func (accumulator *Accumulator) addLogToProducerBatch(topicId string, logData interface{}, callback CallBack) error {
	defer accumulator.lock.Unlock()
	accumulator.lock.Lock()
	// yg-go patch: check under the lock, otherwise a concurrent Close can drain
	// the pending batches between this check and the insert, stranding the new
	// batch. Keep while vendoring.
	if accumulator.shutDownFlag.Load() {
		return errors.New("producer has shutdown and cannot write to new logs")
	}
	if mlog, ok := logData.(*Log); ok {
		if producerBatch, ok := accumulator.logTopicData[topicId]; ok == true {
			logSize, err := GetLogSizeCalculate(mlog)
			if err != nil {
				return err
			}
			asyncAtomic.AddInt64(&producerBatch.totalDataSize, int64(logSize))
			asyncAtomic.AddInt64(&accumulator.producer.producerLogGroupSize, int64(logSize))
			accumulator.addOrSendProducerBatch(topicId, producerBatch, mlog, callback, int64(logSize))
		} else {
			accumulator.createNewProducerBatch(mlog, callback, topicId)
		}
	} else if logList, ok := logData.([]*Log); ok {
		if producerBatch, ok := accumulator.logTopicData[topicId]; ok == true {
			logListSize, err := GetLogListSize(logList)
			if err != nil {
				return err
			}
			asyncAtomic.AddInt64(&producerBatch.totalDataSize, int64(logListSize))
			asyncAtomic.AddInt64(&accumulator.producer.producerLogGroupSize, int64(logListSize))
			accumulator.addOrSendProducerBatch(topicId, producerBatch, logList, callback, int64(logListSize))
		} else {
			accumulator.createNewProducerBatch(logList, callback, topicId)
		}
	} else {
		return errors.New("invalid logType")
	}
	return nil

}
