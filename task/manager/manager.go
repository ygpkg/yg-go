package manager

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/task/model"
	"github.com/ygpkg/yg-go/task/worker"
	"gorm.io/gorm"
)

// Manager 任务管理器实现
type Manager struct {
	config *ManagerConfig
	queue  *Queue
	repo   *TaskRepository

	// 队列同步相关
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
	started bool
	mu      sync.RWMutex
}

// NewManager 创建任务管理器
func NewManager(config *ManagerConfig, db *gorm.DB, redisClient *redis.Client) (*Manager, error) {
	if config == nil {
		config = DefaultManagerConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid manager config: %w", err)
	}

	// 创建队列配置
	queueConfig := &QueueConfig{
		KeyPrefix:   config.KeyPrefix,
		BlockTime:   config.QueueBlockTime,
		RedisClient: redisClient,
		DB:          db,
	}
	queue := NewQueue(queueConfig)

	// 创建仓储
	repo := NewTaskRepository(db)

	return &Manager{
		config: config,
		queue:  queue,
		repo:   repo,
	}, nil
}

// CreateTask 创建任务
func (m *Manager) CreateTask(ctx context.Context, taskEntity *model.TaskEntity) error {
	if err := m.repo.CreateTask(ctx, taskEntity); err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}

	// 推入队列
	if err := m.queue.Push(ctx, taskEntity.TaskType); err != nil {
		return fmt.Errorf("failed to push task to queue: %w", err)
	}

	return nil
}

// CreateTasks 批量创建任务
func (m *Manager) CreateTasks(ctx context.Context, tasks []*model.TaskEntity) error {
	if len(tasks) == 0 {
		return nil
	}

	if err := m.repo.CreateTasks(ctx, tasks); err != nil {
		return fmt.Errorf("failed to batch create tasks: %w", err)
	}

	// 推入第一个任务到队列
	firstTask := tasks[0]
	if err := m.queue.Push(ctx, firstTask.TaskType); err != nil {
		return fmt.Errorf("failed to push first task to queue: %w", err)
	}

	return nil
}

// GetTask 获取任务信息
func (m *Manager) GetTask(ctx context.Context, taskID uint) (*model.TaskEntity, error) {
	return m.repo.GetTaskByID(ctx, taskID)
}

// SaveTask 保存任务（接受 Task 接口以实现解耦）
func (m *Manager) SaveTask(ctx context.Context, task interface{}) error {
	// 类型断言转换为具体类型
	taskEntity, ok := task.(*model.TaskEntity)
	if !ok {
		return fmt.Errorf("invalid task type: expected *model.TaskEntity")
	}
	return m.repo.SaveTask(ctx, taskEntity)
}

// CancelTask 取消任务
func (m *Manager) CancelTask(ctx context.Context, taskID uint, reason string) error {
	return m.repo.CancelTask(ctx, taskID, reason)
}

// PushToQueue 推送任务到队列
func (m *Manager) PushToQueue(ctx context.Context, taskType string) error {
	return m.queue.Push(ctx, taskType)
}

// PopFromQueue 从队列中取出任务
func (m *Manager) PopFromQueue(ctx context.Context, taskType, workerID string) (string, error) {
	return m.queue.Pop(ctx, taskType, workerID)
}

// GetOnePendingTask 获取一个待处理的任务
func (m *Manager) GetOnePendingTask(ctx context.Context, taskType, workerID string) (*model.TaskEntity, error) {
	return m.repo.GetOnePendingTask(ctx, taskType, workerID)
}

// GetPendingTaskCount 获取待处理任务数量
func (m *Manager) GetPendingTaskCount(ctx context.Context, taskType string) (int64, error) {
	return m.repo.GetPendingTaskCount(ctx, taskType)
}

// GetNextStepTasks 获取下一个步骤的任务
func (m *Manager) GetNextStepTasks(ctx context.Context, subjectID uint, appGroup string) ([]*model.TaskEntity, error) {
	return m.repo.GetNextStepTasks(ctx, subjectID, appGroup)
}

// InitTaskDBStatus 初始化数据库中的任务状态
func (m *Manager) InitTaskDBStatus(ctx context.Context) error {
	return m.repo.InitTaskDBStatus(ctx)
}

// CheckAndTimeoutTasks 检查并标记超时任务
func (m *Manager) CheckAndTimeoutTasks(ctx context.Context) error {
	return m.repo.CheckAndTimeoutTasks(ctx)
}

// GetQueue 获取队列实例（内部方法）
func (m *Manager) GetQueue() *Queue {
	return m.queue
}

// GetNextStepPendingTaskTypes 获取下一步骤中需要触发的任务类型列表
func (m *Manager) GetNextStepPendingTaskTypes(ctx context.Context, subjectID uint, appGroup string) ([]string, error) {
	// 获取下一步骤的任务
	nextTasks, err := m.repo.GetNextStepTasks(ctx, subjectID, appGroup)
	if err != nil {
		return nil, err
	}

	// 收集需要触发的任务类型（去重）
	taskTypeSet := make(map[string]struct{})
	for _, taskEntity := range nextTasks {
		// 只返回 pending 状态的任务类型（failed 由重试机制处理）
		if taskEntity.IsPending() {
			taskTypeSet[taskEntity.TaskType] = struct{}{}
		}
	}

	// 转换为列表
	result := make([]string, 0, len(taskTypeSet))
	for taskType := range taskTypeSet {
		result = append(result, taskType)
	}

	return result, nil
}

// GetNextTask 获取下一个待执行任务（阻塞式）- 实现 WorkManager 接口
func (m *Manager) GetNextTask(ctx context.Context, taskType, workerID string) (worker.TaskInfo, error) {
	// 1. 从队列中取出消息（阻塞）
	_, err := m.queue.Pop(ctx, taskType, workerID)
	if err != nil {
		return worker.TaskInfo{}, err
	}

	// 2. 从数据库获取任务并锁定
	taskEntity, err := m.repo.GetOnePendingTask(ctx, taskType, workerID)
	if err != nil || taskEntity == nil {
		return worker.TaskInfo{}, err
	}

	// 3. 转换为 TaskInfo
	return worker.TaskInfo{
		ID:        taskEntity.ID,
		TaskType:  taskEntity.TaskType,
		Payload:   taskEntity.Payload,
		Timeout:   taskEntity.Timeout,
		AppGroup:  taskEntity.AppGroup,
		SubjectID: taskEntity.SubjectID,
		Redo:      taskEntity.Redo,
		MaxRedo:   taskEntity.MaxRedo,
	}, nil
}

// SaveTaskResult 保存任务执行结果并处理任务流转 - 实现 WorkManager 接口
func (m *Manager) SaveTaskResult(ctx context.Context, info worker.TaskInfo, result interface{}, execErr error, onCallback func(context.Context) error) error {
	var taskEntity *model.TaskEntity

	// 在事务中保存任务结果并处理流转
	txErr := m.repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// 1. 获取完整任务实体
		var err error
		taskEntity, err = m.repo.GetTaskByID(ctx, info.ID)
		if err != nil {
			return fmt.Errorf("failed to get task: %w", err)
		}

		// 2. 更新任务状态
		if execErr == nil {
			taskEntity.MarkAsSuccess(toJSON(result))
		} else if execErr == context.DeadlineExceeded {
			taskEntity.MarkAsTimeout()
		} else {
			taskEntity.MarkAsFailed(execErr.Error())
		}

		// 3. 保存任务
		if err := tx.Save(taskEntity).Error; err != nil {
			return fmt.Errorf("failed to save task: %w", err)
		}

		// 4. 在事务内处理任务流转（同步）
		if err := m.handleTaskFlowInTx(ctx, taskEntity); err != nil {
			return fmt.Errorf("failed to handle task flow: %w", err)
		}

		return nil
	})

	if txErr != nil {
		return txErr
	}

	// 5. 事务提交后执行回调（回调不在事务内，业务代码自行管理）
	if onCallback != nil {
		if err := onCallback(ctx); err != nil {
			logs.ErrorContextf(ctx, "[task] callback failed: %v", err)
			// 不返回错误，因为任务状态已保存
		}
	}

	return nil
}

// handleTaskFlowInTx 在事务内处理任务流转
func (m *Manager) handleTaskFlowInTx(ctx context.Context, task *model.TaskEntity) error {
	// 失败重试
	if task.CanRetry() {
		if err := m.queue.Push(ctx, task.TaskType); err != nil {
			return fmt.Errorf("failed to push retry task: %w", err)
		}
		return nil
	}

	// 成功则触发下一个任务
	if task.IsSuccess() {
		if task.AppGroup != "" {
			return m.triggerNextStepTaskInTx(ctx, task)
		} else {
			return m.triggerNextNormalTaskInTx(ctx, task.TaskType)
		}
	}

	return nil
}

// triggerNextStepTaskInTx 在事务内触发下一步骤任务
func (m *Manager) triggerNextStepTaskInTx(ctx context.Context, completedTask *model.TaskEntity) error {
	nextTaskTypes, err := m.GetNextStepPendingTaskTypes(ctx, completedTask.SubjectID, completedTask.AppGroup)
	if err != nil {
		return fmt.Errorf("failed to get next step task types: %w", err)
	}

	if len(nextTaskTypes) == 0 {
		logs.DebugContextf(ctx, "[task] no next step tasks for subject %d, appGroup %s",
			completedTask.SubjectID, completedTask.AppGroup)
		return nil
	}

	// 为每个任务类型推送一条消息
	for _, taskType := range nextTaskTypes {
		if err := m.queue.Push(ctx, taskType); err != nil {
			return fmt.Errorf("failed to push next step task: %w", err)
		}
		logs.InfoContextf(ctx, "[task] triggered next step task, type: %s, subject: %d, appGroup: %s",
			taskType, completedTask.SubjectID, completedTask.AppGroup)
	}

	return nil
}

// triggerNextNormalTaskInTx 在事务内触发下一个普通任务
func (m *Manager) triggerNextNormalTaskInTx(ctx context.Context, taskType string) error {
	count, err := m.repo.GetPendingTaskCount(ctx, taskType)
	if err != nil {
		return fmt.Errorf("failed to get pending task count: %w", err)
	}

	if count > 0 {
		if err := m.queue.Push(ctx, taskType); err != nil {
			return fmt.Errorf("failed to push next normal task: %w", err)
		}
	}

	return nil
}

// toJSON 将对象转换为 JSON 字符串
func toJSON(v interface{}) string {
	if v == nil {
		return ""
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(data)
}

// Start 启动队列同步服务
func (m *Manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("queue syncer already started")
	}

	m.ctx, m.cancel = context.WithCancel(ctx)

	// 启动队列同步协程
	m.startRoutine("queue-syncer", m.queueSyncRoutine)

	m.started = true
	logs.InfoContextf(ctx, "[task] queue syncer started")
	return nil
}

// Stop 停止队列同步服务
func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.started {
		m.mu.Unlock()
		return fmt.Errorf("queue syncer not started")
	}
	m.mu.Unlock()

	logs.InfoContextf(ctx, "[task] stopping queue syncer...")

	// 取消上下文，通知所有协程退出
	m.cancel()

	// 等待所有协程退出（带超时）
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logs.InfoContextf(ctx, "[task] queue syncer goroutines stopped")
	case <-time.After(30 * time.Second):
		logs.WarnContextf(ctx, "[task] queue syncer stop timeout after 30s")
	}

	m.mu.Lock()
	m.started = false
	m.mu.Unlock()

	logs.InfoContextf(ctx, "[task] queue syncer stopped")
	return nil
}

// startRoutine 启动协程的统一封装
func (m *Manager) startRoutine(name string, fn func()) {
	m.wg.Add(1)
	go func() {
		defer m.wg.Done()
		defer func() {
			if r := recover(); r != nil {
				logs.ErrorContextf(m.ctx, "[task] queue syncer routine %s panic: %v", name, r)
			}
		}()
		fn()
	}()
}

// queueSyncRoutine 队列同步协程
func (m *Manager) queueSyncRoutine() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			if err := m.SyncQueueCount(m.ctx); err != nil {
				logs.ErrorContextf(m.ctx, "[task] failed to sync queue count: %v", err)
			}
		}
	}
}

// SyncQueueCount 同步队列数量
// 检查队列中的消息数量，如果有待处理任务则补充消息
func (m *Manager) SyncQueueCount(ctx context.Context) error {
	types, err := m.queue.GetAllTaskTypes(ctx)
	if err != nil {
		logs.ErrorContextf(ctx, "[task] failed to get task types: %v", err)
		return err
	}

	for _, taskType := range types {
		if err := m.syncTaskTypeQueue(ctx, taskType); err != nil {
			logs.ErrorContextf(ctx, "[task] failed to sync queue for task type %s: %v", taskType, err)
		}
	}

	return nil
}

// syncTaskTypeQueue 同步特定任务类型的队列
func (m *Manager) syncTaskTypeQueue(ctx context.Context, taskType string) error {
	// 获取数据库中的任务数量，然后推送消息
	taskCount, err := m.repo.GetPendingTaskCount(ctx, taskType)
	if err != nil {
		return fmt.Errorf("failed to get task count: %w", err)
	}

	// 如果有待处理任务，至少确保队列中有一条消息
	if taskCount > 0 {
		// 简单策略：如果有待处理任务，就推送一条消息
		// 队列中的消息数量由 Worker 消费后自动触发下一条
		if err := m.queue.Push(ctx, taskType); err != nil {
			logs.ErrorContextf(ctx, "[task] failed to push task: %v", err)
		} else {
			logs.DebugContextf(ctx, "[task] synced queue for taskType: %s, pending tasks: %d", taskType, taskCount)
		}
	}

	return nil
}
