package worker

import (
	"time"
)

// WorkerConfig Worker 配置
type WorkerConfig struct {
	// Timeout 默认超时时间
	Timeout time.Duration
	// MaxRedo 默认重试次数
	MaxRedo int
	// MaxConcurrency 最大并发数
	MaxConcurrency int
	// WorkerID Worker 标识
	WorkerID string
}

// DefaultWorkerConfig 默认配置
func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		Timeout:        10 * time.Minute,
		MaxRedo:        3,
		MaxConcurrency: 5,
		WorkerID:       "",
	}
}

// Validate 验证配置
func (c *WorkerConfig) Validate() error {
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
