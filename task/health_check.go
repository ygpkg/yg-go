package task

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

const (
	// HeartbeatTimeout 心跳超时时间（秒）
	HeartbeatTimeout = 30
)

// HealthCheckerConfig 健康检查器配置
type HealthCheckerConfig struct {
	// KeyPrefix Redis 键前缀
	KeyPrefix string
	// RedisClient Redis 客户端
	RedisClient *redis.Client
	// DB 数据库连接
	DB *gorm.DB
	// Queue 任务队列
	Queue *Queue
}

// Validate 验证配置
func (c *HealthCheckerConfig) Validate() error {
	if c.RedisClient == nil {
		return fmt.Errorf("health checker config: redis client is required")
	}
	if c.DB == nil {
		return fmt.Errorf("health checker config: db is required")
	}
	if c.Queue == nil {
		return fmt.Errorf("health checker config: queue is required")
	}
	if c.KeyPrefix == "" {
		return fmt.Errorf("health checker config: key prefix cannot be empty")
	}
	return nil
}

// HealthChecker 健康检查器
type HealthChecker struct {
	config *HealthCheckerConfig
}

// NewHealthChecker 创建健康检查器
func NewHealthChecker(config *HealthCheckerConfig) *HealthChecker {
	if config == nil {
		panic("health checker config is required")
	}
	if err := config.Validate(); err != nil {
		panic(fmt.Sprintf("invalid health checker config: %v", err))
	}

	return &HealthChecker{
		config: config,
	}
}

// getRepo 按需创建 TaskRepository
func (h *HealthChecker) getRepo() *TaskRepository {
	return NewTaskRepository(h.config.DB)
}

// heartbeatKey 获取心跳 key
func (h *HealthChecker) heartbeatKey(taskType string) string {
	return fmt.Sprintf("%stask_heartbeat:%s", h.config.KeyPrefix, taskType)
}

// SetHeartbeat 设置心跳
// 更新 Worker 的心跳时间戳
func (h *HealthChecker) SetHeartbeat(ctx context.Context, taskType, workerID string, taskID uint) error {
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
func (h *HealthChecker) DeleteHeartbeat(ctx context.Context, taskType, workerID string) error {
	key := h.heartbeatKey(taskType)
	_, err := h.config.RedisClient.HDel(ctx, key, workerID).Result()
	if err != nil {
		return fmt.Errorf("failed to delete heartbeat: %w", err)
	}
	return nil
}

// GetWorkerCount 获取 Worker 数量
func (h *HealthChecker) GetWorkerCount(ctx context.Context, taskType string) (int64, error) {
	key := h.heartbeatKey(taskType)
	count, err := h.config.RedisClient.HLen(ctx, key).Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get worker count: %w", err)
	}
	return count, nil
}

// CheckWorkerHealth 检查 Worker 健康状态
// 将超时的 Worker 移除，并将其正在执行的任务标记为失败
func (h *HealthChecker) CheckWorkerHealth(ctx context.Context) error {
	now := time.Now().Unix()

	// 获取所有任务类型
	types, err := h.config.Queue.GetAllTaskTypes(ctx)
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

// checkTaskTypeWorkers 检查特定任务类型的 Worker
func (h *HealthChecker) checkTaskTypeWorkers(ctx context.Context, taskType string, now int64) error {
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
			repo := h.getRepo()
			taskEntity, err := repo.GetTaskByIDAndWorkerID(ctx, uint(taskID), workerID)
			if err != nil {
				logs.WarnContextf(ctx, "[task] failed to get task: %v, taskID: %d, workerID: %s", err, taskID, workerID)
				continue
			}

			if taskEntity.IsRunning() {
				taskEntity.MarkAsFailed("worker heartbeat timeout")
				if err := repo.SaveTask(ctx, taskEntity); err != nil {
					logs.ErrorContextf(ctx, "[task] failed to save task: %v", err)
					continue
				}

				// 重新推入队列（如果还可以重试）
				if taskEntity.CanRetry() {
					if err := h.config.Queue.Push(ctx, taskEntity.TaskType); err != nil {
						logs.ErrorContextf(ctx, "[task] failed to repush task: %v", err)
					}
				}
			}
		}
	}

	return nil
}

// IsWorkerAlive 检查 Worker 是否存活
func (h *HealthChecker) IsWorkerAlive(ctx context.Context, taskType, workerID string) (bool, error) {
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

// SyncQueueCount 同步队列数量
// 检查队列中的消息数量，如果小于待处理任务数，则补充消息
func (h *HealthChecker) SyncQueueCount(ctx context.Context) error {
	types, err := h.config.Queue.GetAllTaskTypes(ctx)
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
func (h *HealthChecker) syncTaskTypeQueue(ctx context.Context, taskType string) error {
	// 获取队列中的消息数量
	queueCount, err := h.config.Queue.GetPendingCount(ctx, taskType)
	if err != nil {
		return fmt.Errorf("failed to get queue count: %w", err)
	}

	// 获取数据库中的待处理任务数量
	repo := h.getRepo()
	taskCount, err := repo.GetPendingTaskCount(ctx, taskType)
	if err != nil {
		return fmt.Errorf("failed to get task count: %w", err)
	}

	// 如果队列数量小于任务数量，补充消息
	if queueCount < taskCount {
		diff := taskCount - queueCount
		for i := int64(0); i < diff; i++ {
			if err := h.config.Queue.Push(ctx, taskType); err != nil {
				logs.ErrorContextf(ctx, "[task] failed to push task: %v", err)
				continue
			}
		}
		logs.InfoContextf(ctx, "[task] synced queue count, taskType: %s, added: %d", taskType, diff)
	}

	return nil
}
