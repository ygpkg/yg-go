package task

import (
	"time"
)

// TaskMode 任务模式
type TaskMode string

const (
	// ModeDistributed 分布式模式 - 基于 Redis Stream 的分布式任务队列
	ModeDistributed TaskMode = "distributed"
	// ModeLocal 本地模式 - 本地协程轮询数据库
	ModeLocal TaskMode = "local"
)

// TaskConfig 任务配置
type TaskConfig struct {
	// Timeout 默认超时时间
	Timeout time.Duration
	// MaxRedo 默认重试次数
	MaxRedo int
	// MaxConcurrency 最大并发数
	MaxConcurrency int
	// PollInterval 轮询间隔（本地模式）
	PollInterval time.Duration
	// QueueBlockTime Redis Stream 阻塞时间（分布式模式）
	QueueBlockTime time.Duration
	// HealthCheckPeriod 健康检查周期（分布式模式）
	HealthCheckPeriod time.Duration
	// RedisKeyPrefix Redis 键前缀
	RedisKeyPrefix string
	// WorkerID Worker 标识（分布式模式）
	WorkerID string
	// EnableHealthCheck 是否启用健康检查（分布式模式）
	EnableHealthCheck bool
}

// DefaultConfig 默认配置
func DefaultConfig() *TaskConfig {
	return &TaskConfig{
		Timeout:           10 * time.Minute,
		MaxRedo:           3,
		MaxConcurrency:    5,
		PollInterval:      5 * time.Second,
		QueueBlockTime:    5 * time.Second, // Redis Stream 阻塞 5 秒
		HealthCheckPeriod: 30 * time.Second,
		RedisKeyPrefix:    "task:",
		WorkerID:          "",
		EnableHealthCheck: true,
	}
}

// Validate 验证配置
func (c *TaskConfig) Validate() error {
	if c.WorkerID == "" {
		return ErrEmptyWorkerID
	}
	if c.Timeout <= 0 {
		return ErrInvalidTimeout
	}
	if c.MaxRedo < 0 {
		return ErrInvalidMaxRedo
	}
	if c.MaxConcurrency <= 0 {
		return ErrInvalidMaxConcurrency
	}
	return nil
}
