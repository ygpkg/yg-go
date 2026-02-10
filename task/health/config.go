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
	// DefaultCheckPeriod 默认健康检查周期
	DefaultCheckPeriod = 30 * time.Second
)

// CheckerConfig 健康检查器配置
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

// DeadWorkerInfo 死亡 Worker 信息
type DeadWorkerInfo struct {
	WorkerID      string
	TaskType      string
	TaskID        uint
	LastHeartbeat int64
}

// DefaultCheckerConfig 返回默认健康检查器配置
func DefaultCheckerConfig() *CheckerConfig {
	return &CheckerConfig{
		KeyPrefix:   "task:",
		CheckPeriod: DefaultCheckPeriod,
	}
}

// Validate 验证配置
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
