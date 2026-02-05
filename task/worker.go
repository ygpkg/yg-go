package task

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// Worker 分布式 Worker 实现
type Worker struct {
	config        *TaskConfig
	db            *gorm.DB
	repo          *TaskRepository
	queue         *Queue
	healthChecker *HealthChecker
	registry      *ExecutorRegistry

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	started bool
	mu      sync.RWMutex
}

// NewWorker 创建分布式 Worker
func NewWorker(config *TaskConfig, db *gorm.DB) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	repo := NewTaskRepository(db)
	queue := NewQueue(config.RedisKeyPrefix)
	healthChecker := NewHealthChecker(config.RedisKeyPrefix, repo, queue)

	return &Worker{
		config:        config,
		db:            db,
		repo:          repo,
		queue:         queue,
		healthChecker: healthChecker,
		registry:      NewExecutorRegistry(),
	}, nil
}

// NewWorkerWithDBInstance 使用数据库实例名称创建分布式 Worker
// dbInstance: 数据库实例名称，通过 dbtools 获取 gorm.DB
func NewWorkerWithDBInstance(config *TaskConfig) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	// 获取数据库实例
	db := dbtools.DB(config.DBInstance)
	if db == nil {
		return nil, fmt.Errorf("database instance not found: %s", config.DBInstance)
	}

	return NewWorker(config, db)
}

// RegisterExecutor 注册任务执行器
func (w *Worker) RegisterExecutor(taskType string, factory ExecutorFactory) {
	w.registry.Register(taskType, factory)
}

// CreateTask 创建任务
func (w *Worker) CreateTask(ctx context.Context, taskEntity *TaskEntity) error {
	// 创建任务记录
	if err := w.repo.CreateTask(ctx, taskEntity); err != nil {
		return err
	}

	// 推入队列
	if err := w.queue.Push(ctx, taskEntity.TaskType); err != nil {
		return err
	}

	logs.InfoContextf(ctx, "[task] created task, id: %d, type: %s", taskEntity.ID, taskEntity.TaskType)
	return nil
}

// BatchCreateTasks 批量创建任务，并将第一个任务推入队列
func (w *Worker) BatchCreateTasks(ctx context.Context, tasks []*TaskEntity) error {
	if len(tasks) == 0 {
		return nil
	}

	// 批量创建任务记录
	if err := w.repo.BatchCreateTasks(ctx, tasks); err != nil {
		return fmt.Errorf("failed to batch create tasks: %w", err)
	}

	// 将第一个任务推入队列
	firstTask := tasks[0]
	if err := w.queue.Push(ctx, firstTask.TaskType); err != nil {
		return fmt.Errorf("failed to push first task to queue: %w", err)
	}

	logs.InfoContextf(ctx, "[task] batch created %d tasks, first task id: %d, type: %s",
		len(tasks), firstTask.ID, firstTask.TaskType)
	return nil
}

// Start 启动 Worker
func (w *Worker) Start(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.started {
		return ErrManagerAlreadyStarted
	}

	// 初始化数据库任务状态
	if err := w.repo.InitTaskDBStatus(ctx); err != nil {
		return fmt.Errorf("failed to init task status: %w", err)
	}

	// 创建上下文
	w.ctx, w.cancel = context.WithCancel(ctx)

	// 启动超时检查协程（仅主节点）
	w.wg.Add(1)
	go w.timeoutCheckRoutine()

	// 启动健康检查协程（仅主节点）
	if w.config.EnableHealthCheck {
		w.wg.Add(1)
		go w.healthCheckRoutine()
	}

	// 为每个注册的任务类型启动工作协程
	taskTypes := w.registry.GetAll()
	for _, taskType := range taskTypes {
		for i := 0; i < w.config.MaxConcurrency; i++ {
			w.wg.Add(1)
			go w.workRoutine(taskType)
		}
		logs.InfoContextf(ctx, "[task] started %d workers for task type: %s", w.config.MaxConcurrency, taskType)
	}

	w.started = true
	logs.InfoContextf(ctx, "[task] worker started, workerID: %s", w.config.WorkerID)
	return nil
}

// Stop 停止 Worker
func (w *Worker) Stop(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if !w.started {
		return ErrManagerNotStarted
	}

	// 取消上下文
	if w.cancel != nil {
		w.cancel()
	}

	// 等待所有协程退出
	w.wg.Wait()

	w.started = false
	logs.InfoContextf(ctx, "[task] worker stopped")
	return nil
}

// GetTask 获取任务信息
func (w *Worker) GetTask(ctx context.Context, taskID uint) (*TaskEntity, error) {
	return w.repo.GetTaskByID(ctx, taskID)
}

// CancelTask 取消任务
func (w *Worker) CancelTask(ctx context.Context, taskID uint, reason string) error {
	return w.repo.CancelTask(ctx, taskID, reason)
}

// PullTask 拉取任务
func (w *Worker) PullTask(ctx context.Context, taskType string) (*TaskEntity, error) {
	// 从队列中取出消息
	_, err := w.queue.Pop(ctx, taskType, w.config.WorkerID)
	if err != nil {
		return nil, err
	}

	// 从数据库获取待处理任务
	task, err := w.repo.GetOnePendingTask(ctx, taskType, w.config.WorkerID)
	if err != nil {
		return nil, err
	}

	return task, nil
}

// ReportHeartbeat 上报心跳
func (w *Worker) ReportHeartbeat(ctx context.Context, taskID uint) error {
	taskEntity, err := w.repo.GetTaskByID(ctx, taskID)
	if err != nil {
		return err
	}
	return w.healthChecker.SetHeartbeat(ctx, taskEntity.TaskType, w.config.WorkerID, taskID)
}

// CompleteTask 完成任务
func (w *Worker) CompleteTask(ctx context.Context, taskEntity *TaskEntity) error {
	return w.repo.SaveTask(ctx, taskEntity)
}

// workRoutine 工作协程
func (w *Worker) workRoutine(taskType string) {
	defer w.wg.Done()

	for {
		select {
		case <-w.ctx.Done():
			logs.InfoContextf(w.ctx, "[task] work routine exit, taskType: %s", taskType)
			return
		case <-time.After(time.Second):
			w.processOneTask(taskType)
		}
	}
}

// processOneTask 处理一个任务
func (w *Worker) processOneTask(taskType string) {
	ctx := logs.WithContextFields(w.ctx, "worker_id", w.config.WorkerID, "task_type", taskType)

	// 拉取任务
	task, err := w.PullTask(ctx, taskType)
	if err != nil {
		logs.ErrorContextf(ctx, "[task] failed to pull task: %v", err)
		return
	}

	if task == nil {
		// 没有待处理任务
		return
	}

	// 处理任务
	w.executeTask(ctx, task)
}

// executeTask 执行任务
func (w *Worker) executeTask(ctx context.Context, taskEntity *TaskEntity) {
	ctx = logs.WithContextFields(ctx, "task_id", taskEntity.ID)

	// 获取执行器
	factory, ok := w.registry.Get(taskEntity.TaskType)
	if !ok {
		logs.ErrorContextf(ctx, "[task] executor not found for task type: %s", taskEntity.TaskType)
		taskEntity.MarkAsFailed("executor not found")
		w.repo.SaveTask(ctx, taskEntity)
		return
	}

	// 创建执行器实例
	executor := factory()

	// Prepare 执行器
	if err := executor.Prepare(ctx, taskEntity); err != nil {
		logs.ErrorContextf(ctx, "[task] failed to setup executor: %v", err)
		taskEntity.MarkAsFailed(fmt.Sprintf("setup failed: %v", err))
		w.repo.SaveTask(ctx, taskEntity)
		return
	}

	// 执行任务（带超时控制）
	execCtx, cancel := context.WithTimeout(ctx, taskEntity.Timeout)
	defer cancel()

	// 启动心跳上报
	heartbeatDone := make(chan struct{})
	go w.heartbeatRoutine(ctx, taskEntity.ID, heartbeatDone)

	// 执行任务
	execDone := make(chan error, 1)
	go func() {
		execDone <- executor.Execute(execCtx)
	}()

	var execErr error
	select {
	case <-execCtx.Done():
		// 超时
		taskEntity.MarkAsTimeout()
		logs.WarnContextf(ctx, "[task] task timeout")
	case execErr = <-execDone:
		// 执行完成
		if execErr != nil {
			taskEntity.MarkAsFailed(execErr.Error())
			logs.ErrorContextf(ctx, "[task] task failed: %v", execErr)
		} else {
			taskEntity.MarkAsSuccess("")
			logs.InfoContextf(ctx, "[task] task success")
		}
	}

	// 停止心跳
	close(heartbeatDone)

	// 保存任务结果
	if err := w.saveTaskResult(ctx, taskEntity, executor); err != nil {
		logs.ErrorContextf(ctx, "[task] failed to save task result: %v", err)
	}

	// 如果任务失败且可以重试，重新推入队列
	if taskEntity.CanRetry() {
		if err := w.queue.Push(ctx, taskEntity.TaskType); err != nil {
			logs.ErrorContextf(ctx, "[task] failed to repush task: %v", err)
		}
	}
}

// saveTaskResult 保存任务结果
func (w *Worker) saveTaskResult(ctx context.Context, taskEntity *TaskEntity, executor TaskExecutor) error {
	return w.db.Transaction(func(tx *gorm.DB) error {
		// 保存任务
		if err := tx.Save(taskEntity).Error; err != nil {
			return err
		}

		// 调用回调
		if taskEntity.IsSuccess() {
			if err := executor.OnSuccess(ctx, tx); err != nil {
				return err
			}
		} else {
			if err := executor.OnFailure(ctx, tx); err != nil {
				return err
			}
		}

		return nil
	})
}

// heartbeatRoutine 心跳协程
func (w *Worker) heartbeatRoutine(ctx context.Context, taskID uint, done chan struct{}) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-done:
			return
		case <-ticker.C:
			if err := w.ReportHeartbeat(ctx, taskID); err != nil {
				logs.ErrorContextf(ctx, "[task] failed to report heartbeat: %v", err)
			}
		}
	}
}

// timeoutCheckRoutine 超时检查协程
func (w *Worker) timeoutCheckRoutine() {
	defer w.wg.Done()

	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			logs.InfoContextf(w.ctx, "[task] timeout check routine exit")
			return
		case <-ticker.C:
			if err := w.repo.CheckAndTimeoutTasks(w.ctx); err != nil {
				logs.ErrorContextf(w.ctx, "[task] failed to check timeout tasks: %v", err)
			}
			if err := w.healthChecker.SyncQueueCount(w.ctx); err != nil {
				logs.ErrorContextf(w.ctx, "[task] failed to sync queue count: %v", err)
			}
		}
	}
}

// healthCheckRoutine 健康检查协程
func (w *Worker) healthCheckRoutine() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.HealthCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			logs.InfoContextf(w.ctx, "[task] health check routine exit")
			return
		case <-ticker.C:
			if err := w.healthChecker.CheckWorkerHealth(w.ctx); err != nil {
				logs.ErrorContextf(w.ctx, "[task] failed to check worker health: %v", err)
			}
		}
	}
}
