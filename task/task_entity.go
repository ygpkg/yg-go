package task

import (
	"time"

	"github.com/ygpkg/yg-go/dbtools"
	"gorm.io/gorm"
)

// TaskStatus 任务状态
type TaskStatus string

const (
	// TaskStatusPending 等待执行
	TaskStatusPending TaskStatus = "pending"
	// TaskStatusRunning 执行中
	TaskStatusRunning TaskStatus = "running"
	// TaskStatusSuccess 执行成功
	TaskStatusSuccess TaskStatus = "success"
	// TaskStatusFailed 执行失败
	TaskStatusFailed TaskStatus = "failed"
	// TaskStatusCanceled 已取消
	TaskStatusCanceled TaskStatus = "canceled"
	// TaskStatusTimeout 超时
	TaskStatusTimeout TaskStatus = "timeout"
)

// TaskEntity 任务模型
type TaskEntity struct {
	gorm.Model
	// CompanyID 公司 ID，用于多租户隔离
	CompanyID uint `gorm:"type:bigint;not null;index:idx_company_id;default:0" json:"company_id" comment:"公司ID"`
	// Uin 用户 ID，用于标识任务归属用户
	Uin uint `gorm:"type:bigint;not null;index:idx_uin;default:0" json:"uin" comment:"用户ID"`
	// SubjectType 主体类型，标识任务关联的业务对象类型（如：order, document, user 等）
	SubjectType string `gorm:"type:varchar(64);not null;index:idx_subject_type" json:"subject_type" comment:"主体类型"`
	// SubjectID 主体 ID，标识任务关联的业务对象 ID
	SubjectID uint `gorm:"type:bigint;not null;index:idx_subject_id" json:"subject_id" comment:"主体ID"`
	// TaskType 任务类型，用于匹配任务执行器
	TaskType string `gorm:"type:varchar(64);not null;index:idx_task_type" json:"task_type" comment:"任务类型"`
	// TaskStatus 任务状态：pending（待执行）、running（执行中）、success（成功）、failed（失败）、canceled（已取消）、timeout（超时）
	TaskStatus TaskStatus `gorm:"type:varchar(20);not null;index:idx_task_status" json:"task_status" comment:"任务状态"`
	// Priority 任务优先级，数值越大优先级越高
	Priority int `gorm:"type:int;not null;default:0;index:idx_priority" json:"priority" comment:"任务优先级"`
	// Step 任务步骤序号，用于步骤化任务执行，前序步骤未完成时后续步骤不会执行
	Step int `gorm:"type:int;not null;default:0" json:"step" comment:"任务步骤序号"`
	// Redo 当前重试次数，每次重试后加 1
	Redo int `gorm:"type:int;not null;default:0" json:"redo" comment:"当前重试次数"`
	// MaxRedo 最大重试次数，当 Redo >= MaxRedo 时不再重试
	MaxRedo int `gorm:"type:int;not null;default:3" json:"max_redo" comment:"最大重试次数"`
	// xTimeout 任务执行超时时间（单位：纳秒），存储为 int64
	Timeout time.Duration `gorm:"type:bigint;not null" json:"timeout" comment:"任务超时时间"`
	// Payload 任务参数，通常为 JSON 格式的业务数据
	Payload string `gorm:"type:text" json:"payload" comment:"任务参数"`
	// Result 任务执行结果，通常为 JSON 格式的返回数据
	Result string `gorm:"type:text" json:"result" comment:"任务执行结果"`
	// ErrMsg 错误信息，任务失败、超时、取消时记录原因
	ErrMsg string `gorm:"type:text" json:"err_msg" comment:"错误信息"`
	// WorkerID Worker 标识，分布式模式下标识处理该任务的 Worker
	WorkerID string `gorm:"type:varchar(64)" json:"worker_id" comment:"Worker标识"`
	// StartAt 任务开始执行时间
	StartAt *time.Time `gorm:"type:datetime" json:"start_at" comment:"开始执行时间"`
	// EndAt 任务结束时间（成功、失败、超时、取消）
	EndAt *time.Time `gorm:"type:datetime" json:"end_at" comment:"结束时间"`
	// Cost 任务执行耗时（单位：秒）
	Cost int64 `gorm:"type:bigint;default:0" json:"cost" comment:"任务执行耗时(秒)"`
	// ParentID 父任务 ID，用于构建父子任务关系
	ParentID uint `gorm:"type:bigint;index:idx_parent_id;default:0" json:"parent_id" comment:"父任务ID"`
	// AppGroup 应用分组，用于将同一业务流程的多个步骤任务组织在一起，支持同一 SubjectID 下多个流程并行
	AppGroup string `gorm:"type:varchar(32);index:idx_app_group" json:"app_group" comment:"应用分组"`
}

type TaskList []TaskEntity

const TableNameCoreTask = "core_task"

// TableName 表名
func (TaskEntity) TableName() string {
	return TableNameCoreTask
}

// InitDB 初始化数据库表
func InitDB(db *gorm.DB) error {
	return dbtools.InitModel(db, &TaskEntity{})
}

// IsPending 是否为待处理状态
func (t *TaskEntity) IsPending() bool {
	return t.TaskStatus == TaskStatusPending
}

// IsRunning 是否为运行中状态
func (t *TaskEntity) IsRunning() bool {
	return t.TaskStatus == TaskStatusRunning
}

// IsFinished 是否已完成（成功、失败、取消、超时）
func (t *TaskEntity) IsFinished() bool {
	return t.TaskStatus == TaskStatusSuccess ||
		t.TaskStatus == TaskStatusFailed ||
		t.TaskStatus == TaskStatusCanceled ||
		t.TaskStatus == TaskStatusTimeout
}

// IsSuccess 是否执行成功
func (t *TaskEntity) IsSuccess() bool {
	return t.TaskStatus == TaskStatusSuccess
}

// CanRetry 是否可以重试
func (t *TaskEntity) CanRetry() bool {
	return t.Redo < t.MaxRedo && (t.TaskStatus == TaskStatusFailed || t.TaskStatus == TaskStatusTimeout)
}

// MarkAsRunning 标记为运行中
func (t *TaskEntity) MarkAsRunning(workerID string) {
	now := time.Now()
	t.TaskStatus = TaskStatusRunning
	t.WorkerID = workerID
	t.StartAt = &now
}

// MarkAsSuccess 标记为成功
func (t *TaskEntity) MarkAsSuccess(result string) {
	now := time.Now()
	t.TaskStatus = TaskStatusSuccess
	t.Result = result
	t.EndAt = &now
	if t.StartAt != nil {
		t.Cost = int64(now.Sub(*t.StartAt).Seconds())
	}
}

// MarkAsFailed 标记为失败
func (t *TaskEntity) MarkAsFailed(errMsg string) {
	now := time.Now()
	t.TaskStatus = TaskStatusFailed
	t.ErrMsg = errMsg
	t.EndAt = &now
	t.Redo++
	if t.StartAt != nil {
		t.Cost = int64(now.Sub(*t.StartAt).Seconds())
	}
}

// MarkAsTimeout 标记为超时
func (t *TaskEntity) MarkAsTimeout() {
	now := time.Now()
	t.TaskStatus = TaskStatusTimeout
	t.ErrMsg = "task execution timeout"
	t.EndAt = &now
	t.Redo++
	if t.StartAt != nil {
		t.Cost = int64(now.Sub(*t.StartAt).Seconds())
	}
}

// MarkAsCanceled 标记为取消
func (t *TaskEntity) MarkAsCanceled(reason string) {
	now := time.Now()
	t.TaskStatus = TaskStatusCanceled
	t.ErrMsg = reason
	t.EndAt = &now
	if t.StartAt != nil {
		t.Cost = int64(now.Sub(*t.StartAt).Seconds())
	}
}

// Validate 验证任务参数
func (t *TaskEntity) Validate() error {
	if t.TaskType == "" {
		return ErrEmptyTaskType
	}
	if t.SubjectID == 0 {
		return ErrInvalidSubjectID
	}
	if t.Payload == "" {
		return ErrEmptyPayload
	}
	if t.Timeout <= 0 {
		return ErrInvalidTimeout
	}
	return nil
}
