package rpcqueue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/ygpkg/yg-go/encryptor"
	"github.com/ygpkg/yg-go/logs"
)

// RPCQueue is the class for all the RPCs
type RPCQueue struct {
	ctx              context.Context
	conn             *amqp.Connection
	rabbitChan       *amqp.Channel
	exchangeName     string
	requestQueueName string
	replyQueue       amqp.Queue
	responseChLocker sync.RWMutex
	responseCh       map[string]chan string
	timeout          time.Duration
}

// NewRPCQueue creates a new RPC Queue
func NewRPCQueue(ctx context.Context, conn *amqp.Connection, exchangeName, reqQueueName string, timeout time.Duration) (rq *RPCQueue, err error) {
	rq = &RPCQueue{
		ctx:              ctx,
		conn:             conn,
		exchangeName:     exchangeName,
		requestQueueName: reqQueueName,
		responseCh:       make(map[string]chan string),
		timeout:          timeout,
	}
	rq.rabbitChan, err = rq.conn.Channel()
	if err != nil {
		logs.ErrorContextf(ctx, "rabbitMq conn err: %v", err)
		return rq, err
	}

	{
		// 创建临时响应队列（自动删除）
		rq.replyQueue, err = rq.rabbitChan.QueueDeclare(
			"", false, true, true, false, nil,
		)
		if err != nil {
			logs.ErrorContextf(ctx, "rabbitMq queue declare failed: %v", err)
			return rq, err
		}
		// 消费回复队列
		msgs, err := rq.rabbitChan.Consume(
			rq.replyQueue.Name, "", true, false, false, false, nil,
		)
		if err != nil {
			logs.ErrorContextf(ctx, "rabbitMq consume reply queue failed: %v", err)
			return rq, err
		}
		go rq.ConsumeReplyRoutine(msgs)
	}
	return rq, err
}

// SendRequest 发送请求，并且监听等待响应。请求和响应都是JSON序列化的对象。
func (rq *RPCQueue) SendRequest(corrID string, queueNamereqBody interface{}) (string, error) {
	if corrID == "" {
		corrID = encryptor.GenerateUUID()
	}
	ch := make(chan string)
	rq.responseChLocker.Lock()
	rq.responseCh[corrID] = ch
	rq.responseChLocker.Unlock()
	defer func() {
		rq.responseChLocker.Lock()
		delete(rq.responseCh, corrID)
		rq.responseChLocker.Unlock()
		close(ch)
	}()

	{
		request, err := json.Marshal(queueNamereqBody)
		if err != nil {
			logs.ErrorContextf(rq.ctx, "marshal request err: %v", err)
			return "", err
		}
		err = rq.rabbitChan.Publish(
			rq.exchangeName, rq.requestQueueName, false, false,
			amqp.Publishing{
				ContentType:   "text/plain",
				CorrelationId: corrID,
				ReplyTo:       rq.replyQueue.Name,
				Body:          []byte(request),
			},
		)
		if err != nil {
			logs.ErrorContextf(rq.ctx, "rabbitMq publish failed: %v", err)
			return "", err
		}
	}

	select {
	case resp := <-ch:
		return resp, nil
	case <-time.After(rq.timeout):
		return "", fmt.Errorf("request queue(%s) timeout", rq.requestQueueName)
	}
}

// ConsumeReplyRoutine 消费响应
func (rq *RPCQueue) ConsumeReplyRoutine(msgs <-chan amqp.Delivery) {
	for {
		select {
		case msg := <-msgs:
			corrID := msg.CorrelationId
			rq.responseChLocker.RLock()
			ch, ok := rq.responseCh[corrID]
			rq.responseChLocker.RUnlock()
			if ok {
				select {
				case ch <- string(msg.Body):
					// 成功写入
				default:
					logs.WarnContextf(rq.ctx, "receive message but send failed(timeout or something), msg=%s", msg.Body)
				}
			}
		case <-rq.ctx.Done():
			logs.Infof("rabbitMQ context.Done(), exit the consume routine")
			return
		}
	}
}
