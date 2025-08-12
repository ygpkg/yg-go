package concqueue

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
)

type Queue interface {
	Submit(t Task)
	StopAndWait() int32
}

// Task 表示一个可执行的任务
type Task func(ctx context.Context) error

// queue 是一个基于生产者-消费者模型的并发控制器
type queue struct {
	taskQueue   chan Task
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	workerCount int
	errCount    int32
	onErr       func(err error) // 处理任务失败时的回调函数
	closed      int32
}

// New 创建一个新的 queue 实例
func New(workerCount, queueSize int, options ...Option) Queue {
	ctx, cancel := context.WithCancel(context.Background())
	q := &queue{
		taskQueue:   make(chan Task, queueSize),
		ctx:         ctx,
		cancel:      cancel,
		workerCount: workerCount,
		onErr: func(err error) {
			fmt.Println(err)
		},
	}
	for _, opt := range options {
		opt(q)
	}
	q.start()
	return q
}

// start 启动 worker 协程（消费者）
func (q *queue) start() {
	// 使用原子操作检查是否已经关闭
	if atomic.LoadInt32(&q.closed) == 1 {
		return
	}
	for i := 0; i < q.workerCount; i++ {
		q.wg.Add(1)
		go q.worker(i)
	}
}

// Submit (生产者)提交一个任务到队列
func (q *queue) Submit(t Task) {
	if atomic.LoadInt32(&q.closed) == 1 {
		// 队列已关闭，直接丢弃任务
		return
	}

	// 没有设置超时时间，则直接提交任务
	select {
	case <-q.ctx.Done():
		// 队列已关闭，丢弃任务
		return
	case q.taskQueue <- t:
		// 任务提交成功
	}
}

// worker 是消费任务的协程
func (q *queue) worker(workerID int) {
	defer q.wg.Done()
	for {
		select {
		case <-q.ctx.Done():
			return
		case task, ok := <-q.taskQueue:
			if !ok {
				return
			}
			q.runTask(workerID, task)
		}
	}
}

func (q *queue) runTask(workerID int, task Task) {
	defer func() {
		if r := recover(); r != nil {
			// 记录panic信息，可选择记录日志
			atomic.AddInt32(&q.errCount, 1) // 将panic也计入错误

			if q.onErr != nil {
				q.onErr(fmt.Errorf("worker %d panic: %v", workerID, r))
			}

			// 可选：重启worker保持池的worker数量
			if atomic.LoadInt32(&q.closed) == 0 {
				q.wg.Add(1)
				go q.worker(workerID) // 重启一个新的worker替代当前崩溃的worker
			}
		}
	}()
	// 执行任务
	if err := task(q.ctx); err != nil {
		atomic.AddInt32(&q.errCount, 1) // 使用原子操作增加失败任务数
	}
}

// StopAndWait 关闭队列并等待所有任务完成
func (q *queue) StopAndWait() int32 {
	q.stop()             // 标记关闭并关闭通道
	errCount := q.wait() // 等待所有worker完成
	q.cancel()           // 所有任务完成后，取消context
	return errCount
}

func (q *queue) stop() {
	if atomic.CompareAndSwapInt32(&q.closed, 0, 1) {
		close(q.taskQueue)
	}
}

func (q *queue) wait() int32 {
	q.wg.Wait()
	return atomic.LoadInt32(&q.errCount)
}
