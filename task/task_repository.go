package task

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TaskRepository 数据访问对象
type TaskRepository struct {
	db      *gorm.DB
	taskDao *TaskDao
}

// NewTaskRepository 创建 TaskRepository
func NewTaskRepository(db *gorm.DB) *TaskRepository {
	return &TaskRepository{
		db:      db,
		taskDao: NewTaskDao(db),
	}
}

// GetOnePendingTask 获取一个待处理的任务并标记为 Running
// 使用数据库锁确保并发安全
func (repo *TaskRepository) GetOnePendingTask(ctx context.Context, taskType, workerID string) (*TaskEntity, error) {
	var taskEntity TaskEntity

	// 使用事务执行查询和更新
	err := repo.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.
			Where("task_type = ?", taskType).
			Where("task_status IN ?", []TaskStatus{TaskStatusPending, TaskStatusFailed}).
			Where("redo < max_redo").
			Where(`
				NOT EXISTS (
					SELECT 1 FROM core_task t2
					WHERE t2.subject_id = core_task.subject_id
					  AND t2.app_group = core_task.app_group
					  AND t2.step < core_task.step
					  AND t2.deleted_at IS NULL
					  AND t2.task_status NOT IN ?
				)
			`, []TaskStatus{TaskStatusCanceled, TaskStatusSuccess}).
			Order("priority DESC, updated_at ASC").
			Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Find(&taskEntity).Error
		// 加锁查询，排除 step 更小但未成功的任务
		if err != nil {
			return fmt.Errorf("failed to find pending task: %w", err)
		}
		if taskEntity.ID == 0 {
			return nil
		}

		// 更新任务状态为 Running
		taskEntity.MarkAsRunning(workerID)
		if err := tx.Save(&taskEntity).Error; err != nil {
			return fmt.Errorf("failed to update task status: %w", err)
		}

		return nil // 返回 nil 表示成功，事务会自动提交
	})

	if err != nil {
		return nil, err
	}

	return &taskEntity, nil
}

// GetTaskByID 根据 ID 获取任务
func (repo *TaskRepository) GetTaskByID(ctx context.Context, id uint) (*TaskEntity, error) {
	taskEntity, err := repo.taskDao.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	if taskEntity.ID == 0 {
		return nil, ErrTaskNotFound
	}
	return taskEntity, nil
}

// GetTaskByIDAndWorkerID 根据 ID 和 WorkerID 获取任务
func (repo *TaskRepository) GetTaskByIDAndWorkerID(ctx context.Context, id uint, workerID string) (*TaskEntity, error) {
	taskEntity, err := repo.taskDao.GetByCond(ctx, &TaskCond{
		ID:       id,
		WorkerID: workerID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get task: %w", err)
	}
	if taskEntity.ID == 0 {
		return nil, ErrTaskNotFound
	}
	return taskEntity, nil
}

// SaveTask 保存任务
func (repo *TaskRepository) SaveTask(ctx context.Context, taskEntity *TaskEntity) error {
	// 使用 UpdateByID 方法更新任务
	return repo.taskDao.UpdateByID(ctx, taskEntity.ID, taskEntity)
}

// CreateTask 创建任务
func (repo *TaskRepository) CreateTask(ctx context.Context, taskEntity *TaskEntity) error {
	if err := taskEntity.Validate(); err != nil {
		return err
	}

	// 设置默认状态
	if taskEntity.TaskStatus == "" {
		taskEntity.TaskStatus = TaskStatusPending
	}

	return repo.taskDao.Insert(ctx, taskEntity)
}

// CreateTasks 批量创建任务
func (repo *TaskRepository) CreateTasks(ctx context.Context, tasks []*TaskEntity) error {
	if len(tasks) == 0 {
		return nil
	}

	// 验证所有任务
	for _, taskEntity := range tasks {
		if err := taskEntity.Validate(); err != nil {
			return fmt.Errorf("task validation failed: %w", err)
		}

		// 设置默认状态
		if taskEntity.TaskStatus == "" {
			taskEntity.TaskStatus = TaskStatusPending
		}
	}

	// 批量插入
	return repo.db.WithContext(ctx).CreateInBatches(tasks, 100).Error
}

// CancelTask 取消任务
func (repo *TaskRepository) CancelTask(ctx context.Context, taskID uint, reason string) error {
	taskEntity, err := repo.GetTaskByID(ctx, taskID)
	if err != nil {
		return err
	}

	// 只能取消未开始或失败的任务
	if !taskEntity.IsPending() && taskEntity.TaskStatus != TaskStatusFailed {
		return fmt.Errorf("cannot cancel task in status: %s", taskEntity.TaskStatus)
	}

	taskEntity.MarkAsCanceled(reason)
	return repo.SaveTask(ctx, taskEntity)
}

// DeleteTask 删除任务（软删除）
func (repo *TaskRepository) DeleteTask(ctx context.Context, id uint) error {
	return repo.taskDao.Delete(ctx, id)
}

// GetPendingTaskCount 获取待处理任务数量
func (repo *TaskRepository) GetPendingTaskCount(ctx context.Context, taskType string) (int64, error) {
	var count int64
	err := repo.db.WithContext(ctx).Model(&TaskEntity{}).
		Where("task_type = ?", taskType).
		Where("task_status IN ?", []TaskStatus{TaskStatusPending, TaskStatusFailed}).
		Where("redo < max_redo").
		Where(`
			NOT EXISTS (
				SELECT 1 FROM core_task t2
				WHERE t2.subject_id = core_task.subject_id
				  AND t2.app_group = core_task.app_group
				  AND t2.step < core_task.step
				  AND t2.deleted_at IS NULL
				  AND t2.task_status NOT IN ?
			)
		`, []TaskStatus{TaskStatusCanceled, TaskStatusSuccess}).
		Count(&count).Error
	return count, err
}

// CheckAndTimeoutTasks 检查并标记超时任务
// 只处理心跳丢失且超时的任务（Worker 崩溃场景）
// healthChecker 用于检查 Worker 心跳状态，如果为 nil 则使用缓冲时间判断
func (repo *TaskRepository) CheckAndTimeoutTasks(ctx context.Context, healthChecker *HealthChecker) error {
	now := time.Now()

	// 查找所有运行中的任务
	tasks, err := repo.taskDao.GetListByCond(ctx, &TaskCond{
		TaskStatus: TaskStatusRunning,
	})
	if err != nil {
		return fmt.Errorf("failed to query running tasks: %w", err)
	}

	var timeoutIDs []uint
	for _, taskEntity := range tasks {
		if taskEntity.StartAt == nil {
			continue
		}

		// 检查任务是否已超时
		timeoutTime := taskEntity.StartAt.Add(taskEntity.Timeout)
		if !now.After(timeoutTime) {
			continue // 未超时
		}

		// 检查 Worker 心跳是否正常
		// 如果心跳正常，说明任务仍在执行中，由执行层处理超时
		if healthChecker != nil {
			isWorkerAlive, err := healthChecker.IsWorkerAlive(ctx, taskEntity.TaskType, taskEntity.WorkerID)
			if err != nil {
				logs.WarnContextf(ctx, "[task] failed to check worker status: %v", err)
				continue
			}

			if isWorkerAlive {
				// Worker 心跳正常，由执行层处理超时
				continue
			}
		} else {
			// 没有 healthChecker，使用缓冲时间判断（2倍心跳周期）
			// 如果任务已超时超过心跳周期*2，说明 Worker 可能已崩溃
			bufferTime := timeoutTime.Add(HeartbeatTimeout * 2 * time.Second)
			if !now.After(bufferTime) {
				continue
			}
		}

		// Worker 心跳丢失或超过缓冲时间，标记任务超时
		timeoutIDs = append(timeoutIDs, taskEntity.ID)
	}

	if len(timeoutIDs) > 0 {
		// 批量更新状态为 timeout
		err = repo.db.WithContext(ctx).Model(&TaskEntity{}).
			Where("id IN ?", timeoutIDs).
			Where("task_status = ?", TaskStatusRunning).
			Updates(map[string]interface{}{
				"task_status": TaskStatusTimeout,
				"err_msg":     "task execution timeout (worker heartbeat lost)",
				"redo":        gorm.Expr("redo + 1"),
				"end_at":      now,
			}).Error

		if err != nil {
			return fmt.Errorf("failed to update timeout tasks: %w", err)
		}

		logs.InfoContextf(ctx, "[task] marked %d tasks as timeout", len(timeoutIDs))
	}

	return nil
}

// InitTaskDBStatus 初始化数据库中的任务状态
// 将所有运行中的任务标记为失败（用于启动时恢复）
func (repo *TaskRepository) InitTaskDBStatus(ctx context.Context) error {
	// 由于需要批量更新并使用 gorm.Expr，这里保留原始实现
	err := repo.db.WithContext(ctx).Model(&TaskEntity{}).
		Where("task_status = ?", TaskStatusRunning).
		Updates(map[string]interface{}{
			"task_status": TaskStatusFailed,
			"err_msg":     "task interrupted by restart",
			"redo":        gorm.Expr("redo + 1"),
		}).Error
	if err != nil {
		return fmt.Errorf("failed to init task status: %w", err)
	}
	return nil
}

// GetNextStepTasks 获取下一个步骤的任务
// 返回第一个未全部完成的 step 中的所有任务
func (repo *TaskRepository) GetNextStepTasks(ctx context.Context, subjectID uint, appGroup string) ([]*TaskEntity, error) {
	// 使用 db 直接查询，因为需要复杂的 WHERE 条件
	var allTasks []TaskEntity
	err := repo.db.WithContext(ctx).
		Where("subject_id = ? AND app_group = ?", subjectID, appGroup).
		Order("step ASC").
		Find(&allTasks).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get tasks: %w", err)
	}

	// 按 step 分组
	stepTaskMap := make(map[int][]*TaskEntity)
	stepSet := map[int]struct{}{}
	for i := range allTasks {
		taskEntity := &allTasks[i]
		stepTaskMap[taskEntity.Step] = append(stepTaskMap[taskEntity.Step], taskEntity)
		stepSet[taskEntity.Step] = struct{}{}
	}

	// 提取并排序所有 step
	var steps []int
	for step := range stepSet {
		steps = append(steps, step)
	}
	sort.Ints(steps)

	// 查找第一个未全部完成的 step
	for _, step := range steps {
		tasks := stepTaskMap[step]
		allCompleted := true
		for _, taskEntity := range tasks {
			if !taskEntity.IsSuccess() {
				allCompleted = false
				break
			}
		}
		if !allCompleted {
			var result []*TaskEntity
			for _, taskEntity := range tasks {
				if taskEntity.IsPending() || taskEntity.TaskStatus == TaskStatusFailed || taskEntity.IsRunning() {
					result = append(result, taskEntity)
				}
			}
			return result, nil
		}
	}

	// 所有任务都完成了
	return nil, nil
}
