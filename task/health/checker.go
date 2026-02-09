package health

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/logs"
)

// Checker 健康检查器
type Checker struct {
	config *CheckerConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	started bool
	mu      sync.RWMutex
}

// NewChecker 创建健康检查器
func NewChecker(config *CheckerConfig) (*Checker, error) {
	if config == nil {
		config = DefaultCheckerConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid health checker config: %w", err)
	}

	return &Checker{
		config: config,
	}, nil
}

// heartbeatKey 获取心跳 key
func (h *Checker) heartbeatKey(taskType string) string {
	return fmt.Sprintf("%stask_heartbeat:%s", h.config.KeyPrefix, taskType)
}

// SetHeartbeat 设置心跳
// 更新 Worker 的心跳时间戳
func (h *Checker) SetHeartbeat(ctx context.Context, taskType, workerID string, taskID uint) error {
	key := h.heartbeatKey(taskType)
	timestamp := time.Now().Unix()
	value := fmt.Sprintf("%d-%d", timestamp, taskID)

	_, err := h.config.RedisClient.HSet(ctx, key, workerID, value).Result()
	if err != nil {
		return fmt.Errorf("failed to set heartbeat: %w", err)
	}

	logs.DebugContextf(ctx, "[task] set heartbeat, taskType: %s, workerID: %s, taskID: %d", taskType, workerID, taskID)
	return nil
}

// DeleteHeartbeat 删除心跳
func (h *Checker) DeleteHeartbeat(ctx context.Context, taskType, workerID string) error {
	key := h.heartbeatKey(taskType)
	_, err := h.config.RedisClient.HDel(ctx, key, workerID).Result()
	if err != nil {
		return fmt.Errorf("failed to delete heartbeat: %w", err)
	}
	return nil
}

// GetWorkerCount 获取 Worker 数量
func (h *Checker) GetWorkerCount(ctx context.Context, taskType string) (int64, error) {
	key := h.heartbeatKey(taskType)
	count, err := h.config.RedisClient.HLen(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get worker count: %w", err)
	}
	return count, nil
}

// IsWorkerAlive 检查 Worker 是否存活
func (h *Checker) IsWorkerAlive(ctx context.Context, taskType, workerID string) (bool, error) {
	key := h.heartbeatKey(taskType)
	value, err := h.config.RedisClient.HGet(ctx, key, workerID).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil // Worker 不存在
		}
		return false, err
	}

	parts := strings.Split(value, "-")
	if len(parts) < 2 {
		return false, nil
	}

	timestamp, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return false, nil
	}

	// 检查心跳是否在有效期内
	return time.Now().Unix()-timestamp <= HeartbeatTimeout, nil
}

// Start 启动健康检查服务
func (h *Checker) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return fmt.Errorf("health checker already started")
	}

	h.ctx, h.cancel = context.WithCancel(ctx)

	// 启动健康检查协程
	h.startRoutine("health-checker", h.healthCheckRoutine)

	// 启动队列同步协程
	h.startRoutine("queue-syncer", h.queueSyncRoutine)

	h.started = true
	logs.InfoContextf(ctx, "[task] health checker started")
	return nil
}

// Stop 停止健康检查服务
func (h *Checker) Stop(ctx context.Context) error {
	h.mu.Lock()
	if !h.started {
		h.mu.Unlock()
		return fmt.Errorf("health checker not started")
	}
	h.mu.Unlock()

	logs.InfoContextf(ctx, "[task] stopping health checker...")

	// 取消上下文，通知所有协程退出
	h.cancel()

	// 等待所有协程退出（带超时）
	done := make(chan struct{})
	go func() {
		h.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logs.InfoContextf(ctx, "[task] all health checker goroutines stopped")
	case <-time.After(30 * time.Second):
		logs.WarnContextf(ctx, "[task] health checker stop timeout after 30s")
	}

	h.mu.Lock()
	h.started = false
	h.mu.Unlock()

	logs.InfoContextf(ctx, "[task] health checker stopped")
	return nil
}

// startRoutine 启动协程的统一封装
func (h *Checker) startRoutine(name string, fn func()) {
	h.wg.Add(1)
	go func() {
		defer h.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logs.ErrorContextf(h.ctx, "[task] health checker routine %s panic: %v", name, r)
			}
		}()
		fn()
	}()
}

// healthCheckRoutine 健康检查协程
func (h *Checker) healthCheckRoutine() {
	ticker := time.NewTicker(h.config.CheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			if err := h.CheckWorkerHealth(h.ctx); err != nil {
				logs.ErrorContextf(h.ctx, "[task] failed to check worker health: %v", err)
			}
		}
	}
}

// queueSyncRoutine 队列同步协程
func (h *Checker) queueSyncRoutine() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			if err := h.SyncQueueCount(h.ctx); err != nil {
				logs.ErrorContextf(h.ctx, "[task] failed to sync queue count: %v", err)
			}
		}
	}
}

// CheckWorkerHealth 检查 Worker 健康状态
// 将超时的 Worker 移除，并将其正在执行的任务标记为失败
func (h *Checker) CheckWorkerHealth(ctx context.Context) error {
	now := time.Now().Unix()

	// 获取所有任务类型
	types, err := h.getAllTaskTypes(ctx)
	if err != nil {
		logs.ErrorContextf(ctx, "[task] failed to get task types: %v", err)
		return err
	}

	for _, taskType := range types {
		if err := h.checkTaskTypeWorkers(ctx, taskType, now); err != nil {
			logs.ErrorContextf(ctx, "[task] failed to check workers for task type %s: %v", taskType, err)
		}
	}

	return nil
}

// getAllTaskTypes 获取所有任务类型（通过扫描 Redis 键）
func (h *Checker) getAllTaskTypes(ctx context.Context) ([]string, error) {
	var keys []string
	var cursor uint64
	pattern := fmt.Sprintf("%stask_queue:*", h.config.KeyPrefix)

	for {
		kk, nextCursor, err := h.config.RedisClient.Scan(ctx, cursor, pattern, 100).Result()
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
	prefix := fmt.Sprintf("%stask_queue:", h.config.KeyPrefix)
	types := make([]string, 0, len(keys))
	for _, key := range keys {
		if len(key) > len(prefix) {
			taskType := key[len(prefix):]
			types = append(types, taskType)
		}
	}

	return types, nil
}

// checkTaskTypeWorkers 检查特定任务类型的 Worker
func (h *Checker) checkTaskTypeWorkers(ctx context.Context, taskType string, now int64) error {
	key := h.heartbeatKey(taskType)

	// 获取所有 Worker 的心跳信息
	workerMap, err := h.config.RedisClient.HGetAll(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to get workers: %w", err)
	}

	for workerID, value := range workerMap {
		parts := strings.Split(value, "-")
		if len(parts) < 2 {
			logs.WarnContextf(ctx, "[task] invalid heartbeat format: %s:%s", workerID, value)
			// 格式不对，删除 Worker
			h.DeleteHeartbeat(ctx, taskType, workerID)
			continue
		}

		timestampStr := parts[0]
		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			logs.WarnContextf(ctx, "[task] failed to parse timestamp: %v, worker: %s", err, workerID)
			// 格式不对，删除 Worker
			h.DeleteHeartbeat(ctx, taskType, workerID)
			continue
		}

		// 判断是否超时
		if now-timestamp > HeartbeatTimeout {
			logs.InfoContextf(ctx, "[task] worker expired: %s, taskType: %s, last heartbeat: %d", workerID, taskType, timestamp)

			// 删除过期的 Worker
			h.DeleteHeartbeat(ctx, taskType, workerID)

			// 标记任务为失败
			taskID, err := strconv.ParseUint(parts[1], 10, 64)
			if err != nil {
				logs.ErrorContextf(ctx, "[task] failed to parse task id: %v, worker: %s", err, workerID)
				continue
			}

			if taskID == 0 {
				continue
			}

			// 获取任务并标记为失败
			taskEntity, err := h.config.Manager.GetTask(ctx, uint(taskID))
			if err != nil {
				logs.WarnContextf(ctx, "[task] failed to get task: %v, taskID: %d, workerID: %s", err, taskID, workerID)
				continue
			}

			if taskEntity.IsRunning() && taskEntity.WorkerID == workerID {
				taskEntity.MarkAsFailed("worker heartbeat timeout")
				if err := h.config.Manager.SaveTask(ctx, taskEntity); err != nil {
					logs.ErrorContextf(ctx, "[task] failed to save task: %v", err)
					continue
				}

				// 重新推入队列（如果还可以重试）
				if taskEntity.CanRetry() {
					if err := h.config.Manager.PushToQueue(ctx, taskEntity.TaskType); err != nil {
						logs.ErrorContextf(ctx, "[task] failed to repush task: %v", err)
					}
				}
			}
		}
	}

	return nil
}

// SyncQueueCount 同步队列数量
// 检查队列中的消息数量，如果小于待处理任务数，则补充消息
func (h *Checker) SyncQueueCount(ctx context.Context) error {
	types, err := h.getAllTaskTypes(ctx)
	if err != nil {
		logs.ErrorContextf(ctx, "[task] failed to get task types: %v", err)
		return err
	}

	for _, taskType := range types {
		if err := h.syncTaskTypeQueue(ctx, taskType); err != nil {
			logs.ErrorContextf(ctx, "[task] failed to sync queue for task type %s: %v", taskType, err)
		}
	}

	return nil
}

// syncTaskTypeQueue 同步特定任务类型的队列
func (h *Checker) syncTaskTypeQueue(ctx context.Context, taskType string) error {
	// 获取数据库中的任务数量，然后推送消息
	taskCount, err := h.config.Manager.GetPendingTaskCount(ctx, taskType)
	if err != nil {
		return fmt.Errorf("failed to get task count: %w", err)
	}

	// 如果有待处理任务，至少确保队列中有一条消息
	if taskCount > 0 {
		// 简单策略：如果有待处理任务，就推送一条消息
		// 队列中的消息数量由 Worker 消费后自动触发下一条
		if err := h.config.Manager.PushToQueue(ctx, taskType); err != nil {
			logs.ErrorContextf(ctx, "[task] failed to push task: %v", err)
		} else {
			logs.DebugContextf(ctx, "[task] synced queue for taskType: %s, pending tasks: %d", taskType, taskCount)
		}
	}

	return nil
}
