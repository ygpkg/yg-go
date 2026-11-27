package tencentcloud_cls_sdk_go

import (
	"sync"
	"time"

	"go.uber.org/atomic"
)

type TimerSendLogTask struct {
	shutDownFlag   *atomic.Bool
	retryQueue     *LogSendRetryQueue
	worker         *Worker
	logAccumulator *Accumulator
	threadPool     *SendThreadPool
	// yg-go patch: closed by notifyShutDown so Close can wake the linger sleep
	// instead of waiting out the remaining LingerMs. Keep while vendoring.
	shutDownNotify chan struct{}
	notifyOnce     sync.Once
}

func NewTimerSendLogTask(logAccumulator *Accumulator, retryQueue *LogSendRetryQueue, ioWorker *Worker, threadPool *SendThreadPool) *TimerSendLogTask {
	timerTask := &TimerSendLogTask{
		shutDownFlag:   atomic.NewBool(false),
		retryQueue:     retryQueue,
		worker:         ioWorker,
		logAccumulator: logAccumulator,
		threadPool:     threadPool,
		shutDownNotify: make(chan struct{}),
	}
	return timerTask
}

// yg-go patch: interruptible replacement for time.Sleep in run().
func (timerTask *TimerSendLogTask) sleep(d time.Duration) {
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timerTask.shutDownNotify:
	case <-timer.C:
	}
}

// yg-go patch: wakes run() so the pending batches are flushed at once.
func (timerTask *TimerSendLogTask) notifyShutDown() {
	timerTask.notifyOnce.Do(func() {
		close(timerTask.shutDownNotify)
	})
}

func (timerTask *TimerSendLogTask) sendToServer(key string, batch *ProducerBatch, config *AsyncProducerClientConfig) {
	if value, ok := timerTask.logAccumulator.logTopicData[key]; !ok {
		return
	} else if GetTimeMs(time.Now().UnixNano())-value.createTimeMs < config.LingerMs {
		return
	}
	timerTask.threadPool.addTask(batch)
	delete(timerTask.logAccumulator.logTopicData, key)
}

func (timerTask *TimerSendLogTask) run(moverWaitGroup *sync.WaitGroup, config *AsyncProducerClientConfig) {
	defer moverWaitGroup.Done()
	for !timerTask.shutDownFlag.Load() {
		sleepMs := config.LingerMs
		nowTimeMs := GetTimeMs(time.Now().UnixNano())
		timerTask.logAccumulator.lock.Lock()
		mapCount := len(timerTask.logAccumulator.logTopicData)
		for key, batch := range timerTask.logAccumulator.logTopicData {
			timeInterval := batch.createTimeMs + config.LingerMs - nowTimeMs
			if timeInterval <= 0 {
				timerTask.sendToServer(key, batch, config)
			} else {
				if sleepMs > timeInterval {
					sleepMs = timeInterval
				}
			}
		}
		timerTask.logAccumulator.lock.Unlock()

		if mapCount == 0 {
			sleepMs = config.LingerMs
		}

		retryProducerBatchList := timerTask.retryQueue.getRetryBatch(timerTask.shutDownFlag.Load())
		if retryProducerBatchList == nil {
			// yg-go patch: was time.Sleep; interruptible so Close does not block.
			timerTask.sleep(time.Duration(sleepMs) * time.Millisecond)
		} else {
			count := len(retryProducerBatchList)
			for i := 0; i < count; i++ {
				timerTask.threadPool.addTask(retryProducerBatchList[i])
			}
		}

	}
	timerTask.logAccumulator.lock.Lock()
	for _, batch := range timerTask.logAccumulator.logTopicData {
		timerTask.threadPool.addTask(batch)
	}
	timerTask.logAccumulator.logTopicData = make(map[string]*ProducerBatch)
	timerTask.logAccumulator.lock.Unlock()

	producerBatchList := timerTask.retryQueue.getRetryBatch(timerTask.shutDownFlag.Load())
	count := len(producerBatchList)
	for i := 0; i < count; i++ {
		timerTask.threadPool.addTask(producerBatchList[i])
	}
}
