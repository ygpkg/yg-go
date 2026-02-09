package task

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
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
func NewWorker(config *TaskConfig, db *gorm.DB, redisClient *redis.Client) (*Worker, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}

	repo := NewTaskRepository(db)

	// 创建队列配置
	queueConfig := &QueueConfig{
		KeyPrefix:   config.RedisKeyPrefix,
		BlockTime:   config.QueueBlockTime,
		RedisClient: redisClient,
		DB:          db,
	}
	queue := NewQueue(queueConfig)

	// 创建健康检查器配置
	healthCheckerConfig := &HealthCheckerConfig{
		KeyPrefix:   config.RedisKeyPrefix,
		RedisClient: redisClient,
		DB:          db,
		Queue:       queue,
	}
	healthChecker := NewHealthChecker(healthCheckerConfig)

	return &Worker{
		config:        config,
		db:            db,
		repo:          repo,
		queue:         queue,
		healthChecker: healthChecker,
		registry:      NewExecutorRegistry(),
	}, nil
}

// RegisterExecutor 注册任务执行器
func (w *Worker) RegisterExecutor(taskType string, factory ExecutorFactory) {
	w.registry.Register(taskType, factory)
}

// CreateTasks 批量创建任务
func (w *Worker) CreateTasks(ctx context.Context, tasks []*TaskEntity) error {
	if len(tasks) == 0 {
		return nil
	}

	if err := w.repo.CreateTasks(ctx, tasks); err != nil {
		return fmt.Errorf("failed to batch create tasks: %w", err)
	}

	firstTask := tasks[0]
	if err := w.queue.Push(ctx, firstTask.TaskType); err != nil {
		return fmt.Errorf("failed to push first task to queue: %w", err)
	}

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

	w.ctx, w.cancel = context.WithCancel(ctx)

	// 启动超时检查
	w.startRoutine("timeout-checker", w.timeoutCheckRoutine)

	// 启动健康检查
	if w.config.EnableHealthCheck {
		w.startRoutine("health-checker", w.healthCheckRoutine)
	}

	// 为每个任务类型启动工作协程
	taskTypes := w.registry.GetAll()
	for _, taskType := range taskTypes {
		for i := 0; i < w.config.MaxConcurrency; i++ {
			w.startRoutine(fmt.Sprintf("worker-%s-%d", taskType, i), func() {
				w.workRoutine(taskType)
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
		return ErrManagerNotStarted
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
	_, err := w.queue.Pop(ctx, taskType, w.config.WorkerID)
	if err != nil {
		return nil, err
	}

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
// 返回值：true 表示处理了任务，false 表示无任务
func (w *Worker) processOneTask(taskType string) bool {
	ctx := logs.WithContextFields(w.ctx, "worker_id", w.config.WorkerID, "task_type", taskType)

	// 拉取任务（阻塞等待）
	task, err := w.PullTask(ctx, taskType)
	if err != nil {
		// 忽略 context 取消的错误
		if !errors.Is(err, context.Canceled) {
			logs.ErrorContextf(ctx, "[task] failed to pull task: %v", err)
		}
		return false
	}

	if task == nil || task.ID == 0 {
		// 没有任务或任务为空，继续循环
		// 这种情况可能是：队列中有消息但数据库中没有可执行的任务（已被其他 worker 处理或因 step 依赖暂不可执行）
		return false
	}

	// 执行任务
	w.executeTask(ctx, task)
	return true
}

// executeTask 执行任务
func (w *Worker) executeTask(ctx context.Context, taskEntity *TaskEntity) {
	// 防御性检查：确保任务对象有效
	if taskEntity == nil || taskEntity.ID == 0 {
		logs.ErrorContextf(ctx, "[task] invalid task entity: task is nil or ID is 0")
		return
	}

	ctx = logs.WithContextFields(ctx, "task_id", taskEntity.ID)

	// 获取执行器
	factory, ok := w.registry.Get(taskEntity.TaskType)
	if !ok {
		logs.ErrorContextf(ctx, "[task] executor not found for task type: %s, task_id: %d", taskEntity.TaskType, taskEntity.ID)
		taskEntity.MarkAsFailed("executor not found")
		w.repo.SaveTask(ctx, taskEntity)
		return
	}

	executor := factory()

	// OnStart 执行器
	if err := executor.OnStart(ctx, taskEntity); err != nil {
		logs.ErrorContextf(ctx, "[task] failed to prepare executor: %v", err)
		taskEntity.MarkAsFailed(fmt.Sprintf("prepare failed: %v", err))
		w.repo.SaveTask(ctx, taskEntity)
		return
	}

	// 创建执行上下文（带超时）
	execCtx, execCancel := context.WithTimeout(ctx, taskEntity.Timeout)
	defer execCancel()

	// 启动心跳
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()

	w.startRoutine("heartbeat", func() {
		w.heartbeatRoutine(heartbeatCtx, taskEntity.ID)
	})

	// 执行任务
	execErr := w.doExecute(execCtx, executor)

	// 根据结果更新任务状态
	if execCtx.Err() == context.DeadlineExceeded {
		taskEntity.MarkAsTimeout()
		logs.WarnContextf(ctx, "[task] task timeout")
	} else if execErr != nil {
		taskEntity.MarkAsFailed(execErr.Error())
		logs.ErrorContextf(ctx, "[task] task failed: %v", execErr)
	} else {
		taskEntity.MarkAsSuccess("")
		logs.InfoContextf(ctx, "[task] task success")
	}

	// 保存结果
	if err := w.saveTaskResult(ctx, taskEntity, executor); err != nil {
		logs.ErrorContextf(ctx, "[task] failed to save task result: %v", err)
	}

	// 失败重试
	if taskEntity.CanRetry() {
		_ = w.pushWithRetry(ctx, taskEntity.TaskType) // 错误已在 pushWithRetry 中记录
		return
	}

	// 只有任务成功时，才主动触发下一个任务
	if taskEntity.IsSuccess() {
		w.triggerNextTask(ctx, taskEntity)
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

// saveTaskResult 保存任务结果
func (w *Worker) saveTaskResult(ctx context.Context, taskEntity *TaskEntity, executor TaskExecutor) error {
	return w.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Save(taskEntity).Error; err != nil {
			return err
		}

		if taskEntity.IsSuccess() {
			return executor.OnSuccess(ctx, tx)
		}
		return executor.OnFailure(ctx, tx)
	})
}

// heartbeatRoutine 心跳协程
func (w *Worker) heartbeatRoutine(ctx context.Context, taskID uint) {
	// 立即发送一次心跳
	w.ReportHeartbeat(ctx, taskID)

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
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
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			if err := w.repo.CheckAndTimeoutTasks(w.ctx, w.healthChecker); err != nil {
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
	ticker := time.NewTicker(w.config.HealthCheckPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			if err := w.healthChecker.CheckWorkerHealth(w.ctx); err != nil {
				logs.ErrorContextf(w.ctx, "[task] failed to check worker health: %v", err)
			}
		}
	}
}

// pushWithRetry 带重试的消息推送
func (w *Worker) pushWithRetry(ctx context.Context, taskType string) error {
	maxRetries := 3
	retryInterval := 100 * time.Millisecond
	var lastErr error

	for i := 0; i < maxRetries; i++ {
		if err := w.queue.Push(ctx, taskType); err != nil {
			lastErr = err
			logs.WarnContextf(ctx, "[task] push retry %d/%d failed: %v", i+1, maxRetries, err)
			if i < maxRetries-1 {
				time.Sleep(retryInterval)
				retryInterval *= 2 // 指数退避
			}
			continue
		}
		return nil // 成功
	}

	// 所有重试失败，记录错误日志
	// SyncQueueCount 会作为最终兜底
	logs.ErrorContextf(ctx, "[task] all push retries failed for taskType: %s", taskType)
	return lastErr
}

// triggerNextTask 触发下一个任务
// 前提条件：当前任务已成功完成
func (w *Worker) triggerNextTask(ctx context.Context, completedTask *TaskEntity) {
	// 场景 1：步骤化任务（有 AppGroup）
	if completedTask.AppGroup != "" {
		w.triggerNextStepTask(ctx, completedTask)
		return
	}

	// 场景 2：普通任务（无 AppGroup）
	w.triggerNextNormalTask(ctx, completedTask.TaskType)
}

// triggerNextStepTask 触发下一个步骤的任务
// 参考 GetNextStepTasks 逻辑：只有当前 step 的所有任务都成功后，才触发下一个 step
func (w *Worker) triggerNextStepTask(ctx context.Context, completedTask *TaskEntity) {
	// 获取下一步待执行的任务
	nextTasks, err := w.repo.GetNextStepTasks(ctx, completedTask.SubjectID, completedTask.AppGroup)
	if err != nil {
		logs.ErrorContextf(ctx, "[task] failed to get next step tasks: %v", err)
		return
	}

	if len(nextTasks) == 0 {
		logs.DebugContextf(ctx, "[task] no next step tasks for subject %d, appGroup %s",
			completedTask.SubjectID, completedTask.AppGroup)
		return
	}

	// 收集需要触发的任务类型（去重）
	taskTypeSet := make(map[string]struct{})
	for _, task := range nextTasks {
		// 只推送 pending 状态的任务（failed 由重试机制处理）
		if task.IsPending() {
			taskTypeSet[task.TaskType] = struct{}{}
		}
	}

	// 为每个任务类型推送一条消息
	for taskType := range taskTypeSet {
		if err := w.pushWithRetry(ctx, taskType); err == nil {
			logs.InfoContextf(ctx, "[task] triggered next step task, type: %s, subject: %d, appGroup: %s",
				taskType, completedTask.SubjectID, completedTask.AppGroup)
		}
		// 错误已在 pushWithRetry 中记录
	}
}

// triggerNextNormalTask 触发下一个普通任务
func (w *Worker) triggerNextNormalTask(ctx context.Context, taskType string) {
	// 检查数据库中是否还有待处理任务
	count, err := w.repo.GetPendingTaskCount(ctx, taskType)
	if err != nil {
		logs.ErrorContextf(ctx, "[task] failed to get pending task count: %v", err)
		return
	}

	if count > 0 {
		_ = w.pushWithRetry(ctx, taskType) // 错误已在 pushWithRetry 中记录
	}
}
