package worker

import (
	"context"
	"time"
)

// WorkerHealth worker 健康状态信息
type WorkerHealth struct {
	WorkerID   string    // Worker 标识
	Timestamp  time.Time // 上报时间
	TaskTypes  []string  // 注册的任务类型列表
	Status     string    // 状态：running、stopped
	CustomData any       // 扩展字段，业务侧可自定义
}

// HealthReporter 健康状态上报接口（由业务侧实现）
type HealthReporter interface {
	// ReportHealth 定时上报 worker 健康状态
	ReportHealth(ctx context.Context, health *WorkerHealth) error
}
