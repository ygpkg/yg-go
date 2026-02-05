package task

import "errors"

var (
	// ErrInvalidMode 无效的任务模式
	ErrInvalidMode = errors.New("task: invalid mode")
	// ErrEmptyWorkerID Worker ID 不能为空
	ErrEmptyWorkerID = errors.New("task: worker id cannot be empty in distributed mode")
	// ErrInvalidTimeout 无效的超时时间
	ErrInvalidTimeout = errors.New("task: invalid timeout")
	// ErrInvalidMaxRedo 无效的重试次数
	ErrInvalidMaxRedo = errors.New("task: invalid max redo")
	// ErrInvalidMaxConcurrency 无效的并发数
	ErrInvalidMaxConcurrency = errors.New("task: invalid max concurrency")
	// ErrExecutorNotFound 执行器未找到
	ErrExecutorNotFound = errors.New("task: executor not found")
	// ErrTaskNotFound 任务未找到
	ErrTaskNotFound = errors.New("task: task not found")
	// ErrManagerNotStarted 管理器未启动
	ErrManagerNotStarted = errors.New("task: manager not started")
	// ErrManagerAlreadyStarted 管理器已启动
	ErrManagerAlreadyStarted = errors.New("task: manager already started")
	// ErrFailedToGetLock 获取锁失败
	ErrFailedToGetLock = errors.New("task: failed to get lock")
	// ErrEmptyTaskType 任务类型不能为空
	ErrEmptyTaskType = errors.New("task: task type cannot be empty")
	// ErrEmptyPayload 任务载荷不能为空
	ErrEmptyPayload = errors.New("task: payload cannot be empty")
	// ErrInvalidSubjectID 无效的主体 ID
	ErrInvalidSubjectID = errors.New("task: invalid subject id")
)
