package worker

import "errors"

var (
	// ErrEmptyWorkerID Worker ID 不能为空
	ErrEmptyWorkerID = errors.New("task: worker id cannot be empty")
	// ErrInvalidTimeout 无效的超时时间
	ErrInvalidTimeout = errors.New("task: invalid timeout")
	// ErrInvalidMaxRedo 无效的重试次数
	ErrInvalidMaxRedo = errors.New("task: invalid max redo")
	// ErrInvalidMaxConcurrency 无效的并发数
	ErrInvalidMaxConcurrency = errors.New("task: invalid max concurrency")
	// ErrWorkerNotStarted Worker 未启动
	ErrWorkerNotStarted = errors.New("task: worker not started")
	// ErrWorkerAlreadyStarted Worker 已启动
	ErrWorkerAlreadyStarted = errors.New("task: worker already started")
	// ErrInvalidHealthReportInterval 无效的健康上报间隔
	ErrInvalidHealthReportInterval = errors.New("task: invalid health report interval")
)
