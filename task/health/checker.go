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
	"github.com/ygpkg/yg-go/mutex"
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

func GetChecker() *Checker {
	if stdChecker == nil {
		panic(fmt.Errorf("health checker is nil"))
	}
	return stdChecker
}

type Checker struct {
	config *CheckerConfig

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	started bool
	mu      sync.RWMutex
}

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

func (h *Checker) heartbeatKey(taskType string) string {
	return fmt.Sprintf("%s_task_heartbeat:%s", h.config.KeyPrefix, taskType)
}

// gracePeriodKey 返回宽限期状态的Redis键
func (h *Checker) gracePeriodKey(taskType string) string {
	return fmt.Sprintf("%s_task_grace_period:%s", h.config.KeyPrefix, taskType)
}

func (h *Checker) SetHeartbeat(ctx context.Context, taskType, workerID string, taskID uint) error {
	heartbeatKey := h.heartbeatKey(taskType)
	timestamp := time.Now().Unix()
	value := fmt.Sprintf("%d-%d", timestamp, taskID)

	_, err := h.config.RedisClient.HSet(ctx, heartbeatKey, workerID, value).Result()
	if err != nil {
		return fmt.Errorf("failed to set heartbeat: %w", err)
	}

	gracePeriodKey := h.gracePeriodKey(taskType)
	if _, err := h.config.RedisClient.HDel(ctx, gracePeriodKey, workerID).Result(); err != nil {
		logs.WarnContextf(ctx, "[task] failed to clear grace period status: %v, worker: %s", err, workerID)
	}

	logs.DebugContextf(ctx, "[task] set heartbeat, taskType: %s, workerID: %s, taskID: %d", taskType, workerID, taskID)
	return nil
}

func (h *Checker) DeleteHeartbeat(ctx context.Context, taskType, workerID string) error {
	heartbeatKey := h.heartbeatKey(taskType)
	_, err := h.config.RedisClient.HDel(ctx, heartbeatKey, workerID).Result()
	if err != nil {
		return fmt.Errorf("failed to delete heartbeat: %w", err)
	}

	gracePeriodKey := h.gracePeriodKey(taskType)
	if _, err := h.config.RedisClient.HDel(ctx, gracePeriodKey, workerID).Result(); err != nil {
		logs.WarnContextf(ctx, "[task] failed to clear grace period status: %v, worker: %s", err, workerID)
	}
	return nil
}

func (h *Checker) GetWorkerCount(ctx context.Context, taskType string) (int64, error) {
	heartbeatKey := h.heartbeatKey(taskType)
	count, err := h.config.RedisClient.HLen(ctx, heartbeatKey).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get worker count: %w", err)
	}
	return count, nil
}

func (h *Checker) IsWorkerAlive(ctx context.Context, taskType, workerID string) (bool, error) {
	heartbeatKey := h.heartbeatKey(taskType)
	value, err := h.config.RedisClient.HGet(ctx, heartbeatKey, workerID).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return false, nil
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

	return time.Now().Unix()-timestamp <= HeartbeatTimeout, nil
}

func (h *Checker) Start(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.started {
		return fmt.Errorf("health checker already started")
	}

	h.ctx, h.cancel = context.WithCancel(ctx)

	h.startRoutine("health-checker", h.healthCheckRoutine)

	h.started = true
	logs.InfoContextf(ctx, "[task] health checker started")
	return nil
}

func (h *Checker) Stop(ctx context.Context) error {
	h.mu.Lock()
	if !h.started {
		h.mu.Unlock()
		return fmt.Errorf("health checker not started")
	}
	h.mu.Unlock()

	logs.InfoContextf(ctx, "[task] stopping health checker...")

	h.cancel()

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

func (h *Checker) healthCheckRoutine() {
	ticker := time.NewTicker(h.config.CheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			logs.InfoContextf(h.ctx, "[task] health check routine stopping...")
			return
		case <-ticker.C:
			if !mutex.IsMaster(mutex.WithMutexKey(h.config.KeyPrefix + "_mutex")) {
				continue
			}
			logs.InfoContextf(h.ctx, "[task] health check routine running...")
			if err := h.CheckWorkerHealth(h.ctx); err != nil {
				logs.ErrorContextf(h.ctx, "[task] failed to check worker health: %v", err)
			}
		}
	}
}

func (h *Checker) CheckWorkerHealth(ctx context.Context) error {
	now := time.Now().Unix()

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

func (h *Checker) getAllTaskTypes(ctx context.Context) ([]string, error) {
	var heartbeatKeys []string
	var cursor uint64
	pattern := fmt.Sprintf("%s_task_heartbeat:*", h.config.KeyPrefix)

	for {
		keys, nextCursor, err := h.config.RedisClient.Scan(ctx, cursor, pattern, 100).Result()
		if err != nil {
			return nil, fmt.Errorf("failed to scan keys: %w", err)
		}

		heartbeatKeys = append(heartbeatKeys, keys...)
		cursor = nextCursor

		if cursor == 0 {
			break
		}
	}

	prefix := fmt.Sprintf("%s_task_heartbeat:", h.config.KeyPrefix)
	types := make([]string, 0, len(heartbeatKeys))
	for _, heartbeatKey := range heartbeatKeys {
		if len(heartbeatKey) > len(prefix) {
			taskType := heartbeatKey[len(prefix):]
			types = append(types, taskType)
		}
	}

	return types, nil
}

// checkTaskTypeWorkers 实现三阶段健康状态降级机制
// 第一阶段 - 健康：心跳在 HeartbeatTimeout 内（30秒）
// 第二阶段 - 宽限期：首次超时检测（30秒~60秒），标记但不删除
// 第三阶段 - 死亡：宽限期内未恢复（60秒+），执行死亡回调
//
// 状态转换：
//   - 健康 → 宽限期：首次心跳超时（>30秒）
//   - 宽限期 → 健康：心跳恢复
//   - 宽限期 → 死亡：60秒后仍未恢复
func (h *Checker) checkTaskTypeWorkers(ctx context.Context, taskType string, now int64) error {
	heartbeatKey := h.heartbeatKey(taskType)
	gracePeriodKey := h.gracePeriodKey(taskType)

	heartbeatMap, err := h.config.RedisClient.HGetAll(ctx, heartbeatKey).Result()
	if err != nil {
		return fmt.Errorf("failed to get workers: %w", err)
	}

	gracePeriodMap, err := h.config.RedisClient.HGetAll(ctx, gracePeriodKey).Result()
	if err != nil {
		logs.WarnContextf(ctx, "[task] failed to get grace period map: %v", err)
		gracePeriodMap = make(map[string]string)
	}

	for workerID, heartbeatValue := range heartbeatMap {
		parts := strings.Split(heartbeatValue, "-")
		if len(parts) < 2 {
			logs.WarnContextf(ctx, "[task] invalid heartbeat format: %s:%s", workerID, heartbeatValue)
			h.DeleteHeartbeat(ctx, taskType, workerID)
			continue
		}

		timestampStr := parts[0]
		timestamp, err := strconv.ParseInt(timestampStr, 10, 64)
		if err != nil {
			logs.WarnContextf(ctx, "[task] failed to parse timestamp: %v, worker: %s", err, workerID)
			h.DeleteHeartbeat(ctx, taskType, workerID)
			continue
		}

		elapsed := now - timestamp

		var taskID uint
		if tid, err := strconv.ParseUint(parts[1], 10, 64); err == nil {
			taskID = uint(tid)
		}

		status := h.getWorkerStatus(elapsed, workerID, gracePeriodMap)

		switch status {
		case WorkerStatusHealthy:
			if _, exists := gracePeriodMap[workerID]; exists {
				logs.InfoContextf(ctx, "[task] worker recovered: %s, taskType: %s", workerID, taskType)
				if _, err := h.config.RedisClient.HDel(ctx, gracePeriodKey, workerID).Result(); err != nil {
					logs.WarnContextf(ctx, "[task] failed to clear grace period status: %v, worker: %s", err, workerID)
				}
			}

		case WorkerStatusGracePeriod:
			if _, exists := gracePeriodMap[workerID]; !exists {
				logs.WarnContextf(ctx, "[task] worker entering grace period: %s, taskType: %s, last heartbeat: %d", workerID, taskType, timestamp)
				gracePeriodValue := fmt.Sprintf("%d-%d", now, timestamp)
				if err := h.config.RedisClient.HSet(ctx, gracePeriodKey, workerID, gracePeriodValue).Err(); err != nil {
					logs.ErrorContextf(ctx, "[task] failed to set grace period status: %v, worker: %s", err, workerID)
				}
			}

		case WorkerStatusDead:
			logs.InfoContextf(ctx, "[task] worker dead: %s, taskType: %s, last heartbeat: %d", workerID, taskType, timestamp)

			if h.config.OnWorkerDead != nil {
				info := DeadWorkerInfo{
					WorkerID:      workerID,
					TaskType:      taskType,
					TaskID:        taskID,
					LastHeartbeat: timestamp,
				}

				if err := h.config.OnWorkerDead(ctx, info); err != nil {
					logs.ErrorContextf(ctx, "[task] worker dead callback failed: %v, worker: %s", err, workerID)
					continue
				}
			}

			if err := h.DeleteHeartbeat(ctx, taskType, workerID); err != nil {
				logs.ErrorContextf(ctx, "[task] failed to delete heartbeat: %v, worker: %s", err, workerID)
			}
		}
	}

	return nil
}

// getWorkerStatus 根据已过期时间和宽限期状态判断 worker 状态
// 返回 WorkerStatus：
//   - Healthy：已过期时间 <= HeartbeatTimeout
//   - GracePeriod：首次超时检测，尚未进入宽限期映射
//   - Dead：宽限期已过期（进入后 > GracePeriodTimeout）
func (h *Checker) getWorkerStatus(elapsed int64, workerID string, graceMap map[string]string) WorkerStatus {
	if elapsed <= HeartbeatTimeout {
		return WorkerStatusHealthy
	}

	gracePeriodValue, inGracePeriod := graceMap[workerID]
	if !inGracePeriod {
		return WorkerStatusGracePeriod
	}

	parts := strings.Split(gracePeriodValue, "-")
	if len(parts) < 1 {
		return WorkerStatusGracePeriod
	}

	graceEnterTime, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return WorkerStatusGracePeriod
	}

	timeInGrace := time.Now().Unix() - graceEnterTime
	if timeInGrace > GracePeriodTimeout {
		return WorkerStatusDead
	}

	return WorkerStatusGracePeriod
}
