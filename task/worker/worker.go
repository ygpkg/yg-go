package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/ygpkg/yg-go/logs"
)

// WorkManager worker 需要的管理器接口（最小化依赖）
type WorkManager interface {
	// GetNextTask 获取下一个待执行任务（阻塞式）
	GetNextTask(ctx context.Context, taskType string, workerID string) (TaskInfo, error)

	// SaveTaskResult 保存任务执行结果
	SaveTaskResult(ctx context.Context, info TaskInfo, result interface{}, err error, onCallback func(context.Context) error) error

	// InitTaskDBStatus 初始化任务状态
	InitTaskDBStatus(ctx context.Context) error
}

// TaskInfo 任务基本信息（纯数据结构）
type TaskInfo struct {
	ID        uint
	TaskType  string
	Payload   string
	Timeout   time.Duration
	AppGroup  string
	SubjectID uint
	Redo      int
	MaxRedo   int
}

// ExecutorOption 执行器注册选项
type ExecutorOption func(*executorOptions)

// executorOptions 执行器选项配置
type executorOptions struct {
	maxConcurrency int
}

// WithConcurrency 设置任务类型的最大并发数
func WithConcurrency(n int) ExecutorOption {
	return func(opts *executorOptions) {
		opts.maxConcurrency = n
	}
}

// Worker 分布式 Worker 实现
type Worker struct {
	config   *WorkerConfig
	manager  WorkManager
	registry *ExecutorRegistry

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	started bool
	mu      sync.RWMutex
}

// NewWorker 创建分布式 Worker
func NewWorker(config *WorkerConfig, mgr WorkManager) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	if mgr == nil {
		return nil, fmt.Errorf("manager is required")
	}

	return &Worker{
		config:   config,
		manager:  mgr,
		registry: NewExecutorRegistry(),
	}, nil
}

// RegisterExecutor 注册任务执行器
// 可通过 WithConcurrency 选项设置该任务类型的最大并发数，不设置时使用全局默认并发数
func (w *Worker) RegisterExecutor(taskType string, factory ExecutorFactory, opts ...ExecutorOption) {
	// 默认选项配置
	options := &executorOptions{
		maxConcurrency: w.config.MaxConcurrency, // 默认使用全局配置
	}

	// 应用选项
	for _, opt := range opts {
		opt(options)
	}

	// 注册执行器（使用 registry 的锁，不需要 Worker 的全局锁）
	w.registry.RegisterWithConcurrency(taskType, factory, options.maxConcurrency)
}

// Start 启动 Worker
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return ErrWorkerAlreadyStarted
	}

	w.ctx, w.cancel = context.WithCancel(ctx)

	// 为每个任务类型启动工作协程（支持不同并发数）
	taskTypes := w.registry.GetAll()
	for _, taskType := range taskTypes {
		// 从 registry 获取并发数（不需要持有 Worker 的锁）
		concurrency := w.registry.GetConcurrency(taskType, w.config.MaxConcurrency)
		taskTypeCopy := taskType // 避免闭包捕获问题
		for i := 0; i < concurrency; i++ {
			w.startRoutine(fmt.Sprintf("worker-%s-%d", taskType, i), func() {
				w.workRoutine(taskTypeCopy)
			})
		}
	}

	w.started = true
	logs.InfoContextf(ctx, "[task] worker started, workerID: %s", w.config.WorkerID)
	return nil
}

// startRoutine 启动协程的统一封装
func (w *Worker) startRoutine(name string, fn func()) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logs.ErrorContextf(w.ctx, "[task] routine %s panic: %v", name, r)
			}
		}()
		fn()
	}()
}

// Stop 停止 Worker
func (w *Worker) Stop(ctx context.Context) error {
	w.mu.Lock()
	if !w.started {
		w.mu.Unlock()
		return ErrWorkerNotStarted
	}
	w.mu.Unlock()

	logs.InfoContextf(ctx, "[task] stopping worker...")

	// 取消上下文，通知所有协程退出
	w.cancel()

	// 等待所有协程退出（带超时）
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logs.InfoContextf(ctx, "[task] all goroutines stopped")
	case <-time.After(30 * time.Second):
		logs.WarnContextf(ctx, "[task] stop timeout after 30s")
	}

	w.mu.Lock()
	w.started = false
	w.mu.Unlock()

	logs.InfoContextf(ctx, "[task] worker stopped")
	return nil
}

// workRoutine 工作协程（优化版：使用阻塞式消费）
func (w *Worker) workRoutine(taskType string) {
	for {
		select {
		case <-w.ctx.Done():
			return
		default:
			w.processOneTask(taskType)
		}
	}
}

// processOneTask 处理一个任务
func (w *Worker) processOneTask(taskType string) {
	ctx := logs.WithContextFields(w.ctx, "worker_id", w.config.WorkerID, "task_type", taskType)

	// 从 manager 获取任务（阻塞等待）
	taskInfo, err := w.manager.GetNextTask(ctx, taskType, w.config.WorkerID)
	if err != nil {
		// 忽略 context 取消的错误
		if !errors.Is(err, context.Canceled) {
			logs.ErrorContextf(ctx, "[task] failed to get next task: %v", err)
		}
		return
	}

	if taskInfo.ID == 0 {
		// 没有任务，继续循环
		return
	}

	// 执行任务
	w.executeTask(ctx, taskInfo)
}

// executeTask 执行任务
func (w *Worker) executeTask(ctx context.Context, info TaskInfo) {
	// 防御性检查：确保任务对象有效
	if info.ID == 0 {
		logs.ErrorContextf(ctx, "[task] invalid task info: ID is 0")
		return
	}

	ctx = logs.WithContextFields(ctx, "task_id", info.ID)

	// 获取执行器
	factory, ok := w.registry.Get(info.TaskType)
	if !ok {
		logs.ErrorContextf(ctx, "[task] executor not found for task type: %s, task_id: %d", info.TaskType, info.ID)
		w.manager.SaveTaskResult(ctx, info, nil, fmt.Errorf("executor not found"), nil)
		return
	}

	// 通过工厂函数创建执行器，传入payload由业务层决定如何解析
	executor, err := factory(info.Payload)
	if err != nil {
		logs.ErrorContextf(ctx, "[task] failed to create executor: %v", err)
		w.manager.SaveTaskResult(ctx, info, nil, fmt.Errorf("create executor failed: %w", err), nil)
		return
	}

	// 创建执行上下文（带超时）
	execCtx, execCancel := context.WithTimeout(ctx, info.Timeout)
	defer execCancel()

	// 执行任务
	execErr := w.doExecute(execCtx, executor)

	// 获取执行结果
	result := executor.GetResult()

	// 确定回调函数
	var callback func(context.Context) error
	if execErr == nil && execCtx.Err() != context.DeadlineExceeded {
		callback = executor.OnSuccess
		logs.InfoContextf(ctx, "[task] task success")
	} else {
		callback = executor.OnFailure
		if execCtx.Err() == context.DeadlineExceeded {
			execErr = context.DeadlineExceeded
			logs.WarnContextf(ctx, "[task] task timeout")
		} else {
			logs.ErrorContextf(ctx, "[task] task failed: %v", execErr)
		}
	}

	// 保存结果（manager 内部处理流转）
	if err := w.manager.SaveTaskResult(ctx, info, result, execErr, callback); err != nil {
		logs.ErrorContextf(ctx, "[task] failed to save task result: %v", err)
	}
}

// doExecute 执行任务（带 panic 恢复）
func (w *Worker) doExecute(ctx context.Context, executor TaskExecutor) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("task panic: %v", r)
		}
	}()

	return executor.Execute(ctx)
}
