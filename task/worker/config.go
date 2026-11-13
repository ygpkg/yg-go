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
	// HealthReportInterval 健康上报间隔，0 表示不上报
	HealthReportInterval time.Duration
	// IdleInterval 空闲退避间隔：队列为空或取任务失败时，工作协程等待该时长后再继续，
	// 避免空队列热循环打库。0 表示不退避（与旧版本行为一致）。
	IdleInterval time.Duration
}

// DefaultWorkerConfig 默认配置
func DefaultWorkerConfig() *WorkerConfig {
	return &WorkerConfig{
		Timeout:              10 * time.Minute,
		MaxRedo:              3,
		MaxConcurrency:       5,
		WorkerID:             "",
		HealthReportInterval: 0,
		IdleInterval:         0,
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
	if c.HealthReportInterval < 0 {
		return ErrInvalidHealthReportInterval
	}
	if c.IdleInterval < 0 {
		return ErrInvalidIdleInterval
	}
	return nil
}
