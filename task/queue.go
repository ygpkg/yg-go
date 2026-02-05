package task

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/logs"
)

// Queue Redis Stream 队列
type Queue struct {
	keyPrefix string
}

// NewQueue 创建队列
func NewQueue(keyPrefix string) *Queue {
	return &Queue{
		keyPrefix: keyPrefix,
	}
}

// streamKey 获取 stream 的 key
func (q *Queue) streamKey(taskType string) string {
	return fmt.Sprintf("%stask_queue:%s", q.keyPrefix, taskType)
}

// groupKey 获取 consumer group 的 key
func (q *Queue) groupKey(taskType string) string {
	return fmt.Sprintf("%stask_group:%s", q.keyPrefix, taskType)
}

// Push 将任务推入队列
func (q *Queue) Push(ctx context.Context, taskType string) error {
	stream := q.streamKey(taskType)
	msgID, err := redispool.Redis().XAdd(ctx, &redis.XAddArgs{
		Stream: stream,
		Values: map[string]interface{}{"task_type": taskType, "timestamp": time.Now().Unix()},
	}).Result()
	if err != nil {
		return fmt.Errorf("failed to push task to queue: %w", err)
	}
	logs.DebugContextf(ctx, "[task] push task to queue, taskType: %s, msgID: %s", taskType, msgID)
	return nil
}

// Pop 从队列中取出任务
// 使用 Redis Stream Consumer Group 实现分布式消费
func (q *Queue) Pop(ctx context.Context, taskType, workerID string) (string, error) {
	stream := q.streamKey(taskType)
	group := q.groupKey(taskType)

	// 创建消费组（如果不存在）
	_ = redispool.Redis().XGroupCreateMkStream(ctx, stream, group, "$").Err()

	// 阻塞读取消息
	res, err := redispool.Redis().XReadGroup(ctx, &redis.XReadGroupArgs{
		Group:    group,
		Consumer: workerID,
		Streams:  []string{stream, ">"},
		Count:    1,
		Block:    time.Millisecond * 100, // 阻塞 100ms
		NoAck:    true,                   // 自动确认
	}).Result()

	if err != nil {
		if errors.Is(err, redis.Nil) {
			// 没有消息，返回空
			return "", nil
		}
		return "", fmt.Errorf("failed to pop task from queue: %w", err)
	}

	if len(res) > 0 && len(res[0].Messages) > 0 {
		msgID := res[0].Messages[0].ID
		logs.DebugContextf(ctx, "[task] pop task from queue, taskType: %s, workerID: %s, msgID: %s", taskType, workerID, msgID)
		return msgID, nil
	}

	return "", nil
}

// Ack 确认消费任务
func (q *Queue) Ack(ctx context.Context, taskType, msgID string) error {
	stream := q.streamKey(taskType)
	group := q.groupKey(taskType)
	err := redispool.Redis().XAck(ctx, stream, group, msgID).Err()
	if err != nil {
		return fmt.Errorf("failed to ack task: %w", err)
	}
	return nil
}

// GetPendingCount 获取待处理的消息数量
func (q *Queue) GetPendingCount(ctx context.Context, taskType string) (int64, error) {
	stream := q.streamKey(taskType)
	group := q.groupKey(taskType)

	// 获取 last-delivered-id
	groupInfo, err := redispool.Redis().XInfoGroups(ctx, stream).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get group info: %w", err)
	}

	var lastID string
	found := false
	for _, g := range groupInfo {
		if g.Name == group {
			lastID = g.LastDeliveredID
			found = true
			break
		}
	}
	if !found {
		return 0, nil
	}

	// 获取从 last-delivered-id 到末尾的消息数
	entries, err := redispool.Redis().XRangeN(ctx, stream, lastID, "+", 10000).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get pending count: %w", err)
	}

	// 减掉起始 ID 自己
	count := int64(len(entries))
	if count > 0 {
		count--
	}
	return count, nil
}

// CheckPendingMessages 检查未处理的任务消息
// 将超过指定时间未处理的消息重新推入队列
func (q *Queue) CheckPendingMessages(ctx context.Context, taskType string, idleTime time.Duration) (int, error) {
	stream := q.streamKey(taskType)
	group := q.groupKey(taskType)

	pending, err := redispool.Redis().XPendingExt(ctx, &redis.XPendingExtArgs{
		Stream: stream,
		Group:  group,
		Start:  "-",
		End:    "+",
		Idle:   idleTime,
	}).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to check pending messages: %w", err)
	}

	for _, p := range pending {
		// 重新推入任务队列
		if err := q.Push(ctx, taskType); err != nil {
			logs.ErrorContextf(ctx, "[task] failed to repush task: %v", err)
			continue
		}
		// 确认旧消息
		if err := redispool.Redis().XAck(ctx, stream, group, p.ID).Err(); err != nil {
			logs.ErrorContextf(ctx, "[task] failed to ack old message: %v", err)
			continue
		}
	}

	return len(pending), nil
}

// GetAllTaskTypes 获取所有任务类型
func (q *Queue) GetAllTaskTypes(ctx context.Context) ([]string, error) {
	var keys []string
	var cursor uint64
	pattern := fmt.Sprintf("%stask_queue:*", q.keyPrefix)

	for {
		kk, nextCursor, err := redispool.Redis().Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan keys: %w", err)
		}

		keys = append(keys, kk...)
		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	// 提取任务类型
	prefix := fmt.Sprintf("%stask_queue:", q.keyPrefix)
	types := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(key) > len(prefix) {
			taskType := key[len(prefix):]
			types = append(types, taskType)
		}
	}

	return types, nil
}
