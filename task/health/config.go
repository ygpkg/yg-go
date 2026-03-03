package health

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	// HeartbeatTimeout 心跳超时时间（秒）
	HeartbeatTimeout = 30
	// GracePeriodTimeout 宽限期时间（秒），超过心跳超时时间但未达到宽限期时间的 worker 处于宽限期状态
	GracePeriodTimeout = 60
	DefaultCheckPeriod = 30 * time.Second
)

type WorkerStatus int

const (
	WorkerStatusHealthy WorkerStatus = iota
	WorkerStatusGracePeriod
	WorkerStatusDead
)

func (s WorkerStatus) String() string {
	switch s {
	case WorkerStatusHealthy:
		return "healthy"
	case WorkerStatusGracePeriod:
		return "grace_period"
	case WorkerStatusDead:
		return "dead"
	default:
		return "unknown"
	}
}

type CheckerConfig struct {
	// KeyPrefix Redis 键前缀
	KeyPrefix string
	// RedisClient Redis 客户端
	RedisClient *redis.Client
	// CheckPeriod 健康检查周期
	CheckPeriod time.Duration

	// OnWorkerDead 发现 Worker 死亡时的回调
	// 返回 error 会阻止删除心跳
	OnWorkerDead func(ctx context.Context, info DeadWorkerInfo) error
}
type DeadWorkerInfo struct {
	WorkerID      string
	TaskType      string
	TaskID        uint
	LastHeartbeat int64
}

func DefaultCheckerConfig() *CheckerConfig {
	return &CheckerConfig{
		KeyPrefix:   "task:",
		CheckPeriod: DefaultCheckPeriod,
	}
}

func (c *CheckerConfig) Validate() error {
	if c.RedisClient == nil {
		return fmt.Errorf("health checker config: redis client is required")
	}
	if c.KeyPrefix == "" {
		return fmt.Errorf("health checker config: key prefix cannot be empty")
	}
	if c.CheckPeriod <= 0 {
		c.CheckPeriod = DefaultCheckPeriod
	}
	return nil
}
