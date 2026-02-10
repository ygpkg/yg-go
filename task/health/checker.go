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

var (
	stdChecker *Checker
	once       sync.Once
)

func InitChecker(config *CheckerConfig) error {
	var err error
	once.Do(func() {
		stdChecker, err = NewChecker(config)
	})
	return err
}

// GetChecker 获取全局健康检查器，如果未初始化则 panic
func GetChecker() *Checker {
	if stdChecker == nil {
		panic(fmt.Errorf("health checker is nil"))
	}
	return stdChecker
}

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

			// 解析 TaskID
			taskID, err := strconv.ParseUint(parts[1], 10, 64)
			if err != nil {
				logs.ErrorContextf(ctx, "[task] failed to parse task id: %v, worker: %s", err, workerID)
				// 解析失败也认为是无效心跳，可以删除
				h.DeleteHeartbeat(ctx, taskType, workerID)
				continue
			}

			// 如果配置了回调，则执行回调
			if h.config.OnWorkerDead != nil {
				info := DeadWorkerInfo{
					WorkerID:      workerID,
					TaskType:      taskType,
					TaskID:        uint(taskID),
					LastHeartbeat: timestamp,
				}

				if err := h.config.OnWorkerDead(ctx, info); err != nil {
					logs.ErrorContextf(ctx, "[task] worker dead callback failed: %v, worker: %s", err, workerID)
					// 回调失败，暂时保留心跳，下次重试
					continue
				}
			}

			// 删除过期的 Worker
			if err := h.DeleteHeartbeat(ctx, taskType, workerID); err != nil {
				logs.ErrorContextf(ctx, "[task] failed to delete heartbeat: %v, worker: %s", err, workerID)
			}
		}
	}

	return nil
}
