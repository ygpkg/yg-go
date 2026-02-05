package task

import (
	"context"
	"time"
)

// TaskManager 任务管理器接口
type TaskManager interface {
	// RegisterExecutor 注册任务执行器
	// taskType: 任务类型
	// factory: 执行器工厂函数
	RegisterExecutor(taskType string, factory ExecutorFactory)

	// CreateTask 创建任务
	CreateTask(ctx context.Context, task *TaskEntity) error

	// Start 启动任务管理器
	Start(ctx context.Context) error

	// Stop 停止任务管理器
	Stop(ctx context.Context) error

	// GetTask 获取任务信息
	GetTask(ctx context.Context, taskID uint) (*TaskEntity, error)

	// CancelTask 取消任务
	CancelTask(ctx context.Context, taskID uint, reason string) error
}

// DistributedWorker 分布式 Worker 接口
type DistributedWorker interface {
	TaskManager

	// PullTask 拉取任务
	// taskType: 任务类型
	// 返回待执行的任务，如果没有任务则返回 nil
	PullTask(ctx context.Context, taskType string) (*TaskEntity, error)

	// ReportHeartbeat 上报心跳
	// taskID: 当前正在执行的任务 ID
	ReportHeartbeat(ctx context.Context, taskID uint) error

	// CompleteTask 完成任务
	// 由 Worker 调用来保存任务结果
	CompleteTask(ctx context.Context, task *TaskEntity) error
}

// LocalScheduler 本地调度器接口
type LocalScheduler interface {
	TaskManager

	// SetConcurrency 设置并发数
	SetConcurrency(max int)

	// SetTimeout 设置超时时间
	SetTimeout(timeout time.Duration)

	// GetPendingCount 获取待处理任务数量
	GetPendingCount(ctx context.Context, taskType string) (int64, error)
}
