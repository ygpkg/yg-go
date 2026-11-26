package db

import "time"

// 表名常量。两表都以 id 为主键；core_async_task_run.async_task_id 是逻辑关联，无物理外键。
const (
	// CoreAsyncTaskTableName 任务实体表。
	CoreAsyncTaskTableName = "core_async_task"
	// CoreAsyncTaskRunTableName 单次尝试记录表。
	CoreAsyncTaskRunTableName = "core_async_task_run"
)

// CoreAsyncTask 是有状态的任务实体，对应表 core_async_task。
//
// 与契约「数据模型」字段表逐列对齐；表结构真源是 asynctask/ddl 的 SQL 文件，本结构体
// 只负责读写映射（ddl 对账测试会双向比对列名与索引名）：
//   - 时间列不写 type:datetime(3)（写了 PG 会生成非法类型）；
//   - 计数列用 int32 → MySQL int（Go int 会映射成 bigint），timeout_ms/duration_ms 显式 int64；
//   - uniq_key 用 *string：空即 NULL，不参与唯一约束；唯一性只在活跃期
//     （DDL 侧的 uk_core_async_task_uniq_active：PG/SQLite 用部分唯一索引，
//     MySQL 用生成列 uniq_key_active），所以这里**不加** uniqueIndex tag——
//     加了会让任何 AutoMigrate 重建出「全局唯一」，把终态后重建的语义钉死；
//   - 无软删除（不做 gorm.DeletedAt）；
//   - 索引名一律带表前缀（PG 索引名 schema 级唯一）。
type CoreAsyncTask struct {
	ID string `gorm:"column:id;primaryKey;type:varchar(36);comment:任务 ID（UUID v4）"`
	// created_at / updated_at 由 GORM 维护（可空）。created_at 单独参与
	// idx_core_async_task_task_type_status_created_at（管理面按类型+状态倒序分页）。
	CreatedAt time.Time `gorm:"column:created_at;index:idx_core_async_task_task_type_status_created_at,priority:3"`
	UpdatedAt time.Time `gorm:"column:updated_at"`

	TaskType string `gorm:"column:task_type;type:varchar(128);not null;index:idx_core_async_task_claim,priority:1;index:idx_core_async_task_task_type_status_created_at,priority:1;comment:任务类型"`
	Status   string `gorm:"column:status;type:varchar(16);not null;default:'pending';index:idx_core_async_task_claim,priority:2;index:idx_core_async_task_status_lease_until,priority:1;index:idx_core_async_task_status_finished_at,priority:1;index:idx_core_async_task_task_type_status_created_at,priority:2;comment:任务状态"`
	// Priority 参与认领索引（priority DESC）：认领 SQL 按 `priority DESC, next_run_at` 取任务，
	// 索引顺序与之完全一致才能免掉 filesort。
	Priority int32   `gorm:"column:priority;not null;default:0;index:idx_core_async_task_claim,priority:3,sort:desc;comment:优先级"`
	Payload  string  `gorm:"column:payload;type:text;comment:任务入参"`
	UniqKey  *string `gorm:"column:uniq_key;type:varchar(128);comment:幂等键（仅活跃期唯一）"`
	BizType  string  `gorm:"column:biz_type;type:varchar(64);not null;default:'';index:idx_core_async_task_biz_type_biz_id,priority:1;comment:业务类型"`
	BizID    string  `gorm:"column:biz_id;type:varchar(64);not null;default:'';index:idx_core_async_task_biz_type_biz_id,priority:2;comment:业务 ID"`

	// 一等维度（C2）：租户与业务主体。原先只能塞进 labels（text、无索引，只能全表扫），
	// 现在进独立列并各带索引；新代码禁止再读 labels 里的 company_id。
	CompanyID   uint64 `gorm:"column:company_id;not null;default:0;index:idx_core_async_task_company_status_created,priority:1;comment:租户 ID（0=系统/无租户）"`
	SubjectKind string `gorm:"column:subject_kind;type:varchar(64);not null;default:'';index:idx_core_async_task_subject,priority:1;comment:业务主体类型"`
	SubjectID   string `gorm:"column:subject_id;type:varchar(64);not null;default:'';index:idx_core_async_task_subject,priority:2;comment:业务主体 ID"`

	// 任务链（步骤化）。独立任务：chain_id=''、step=0、prev_step=-1。
	ChainID  string `gorm:"column:chain_id;type:varchar(36);not null;default:'';index:idx_core_async_task_chain_id_step,priority:1;comment:任务链 ID（空=独立任务）"`
	Step     int32  `gorm:"column:step;not null;default:0;index:idx_core_async_task_chain_id_step,priority:2;comment:步骤序号"`
	PrevStep int32  `gorm:"column:prev_step;not null;default:-1;comment:前序步骤序号（-1=首步）"`

	Generation int32 `gorm:"column:generation;not null;default:1;comment:重放轮次"`
	// Attempt 单调执行序号：每次认领 +1，一次执行一行 run（(id,generation,attempt) 唯一）。
	Attempt int32 `gorm:"column:attempt;not null;default:0;comment:本轮执行序号（单调）"`
	// FailureCount 可重试失败次数（准入条件）：暂停不加、可重试失败与失权回收各 +1。
	// 与 attempt 分开是为了让「主动让出」不消耗失败额度，同时保住 run 的唯一标识。
	FailureCount int32 `gorm:"column:failure_count;not null;default:0;comment:可重试失败次数"`
	MaxAttempts  int32 `gorm:"column:max_attempts;not null;default:3;comment:本轮最大失败重试次数"`
	TimeoutMs    int64 `gorm:"column:timeout_ms;not null;default:0;comment:单次执行超时毫秒"`

	NextRunAt  time.Time  `gorm:"column:next_run_at;not null;index:idx_core_async_task_claim,priority:4;comment:最早可认领时间"`
	LeaseUntil *time.Time `gorm:"column:lease_until;index:idx_core_async_task_status_lease_until,priority:2;comment:租约到期时间"`
	WorkerID   string     `gorm:"column:worker_id;type:varchar(64);not null;default:'';comment:持有租约的执行者"`
	StartedAt  *time.Time `gorm:"column:started_at;comment:本轮首次开始时间"`
	FinishedAt *time.Time `gorm:"column:finished_at;index:idx_core_async_task_status_finished_at,priority:2;comment:进入终态的时间"`

	DurationMs int64  `gorm:"column:duration_ms;not null;default:0;comment:累计耗时毫秒"`
	LastError  string `gorm:"column:last_error;type:text;comment:最后一次错误"`
	Result     string `gorm:"column:result;type:text;comment:结果快照"`
	RequestID  string `gorm:"column:request_id;type:varchar(64);not null;default:'';comment:请求 ID"`
	TraceID    string `gorm:"column:trace_id;type:varchar(64);not null;default:'';comment:TraceID"`
	// Labels 开放业务维度（JSON 对象，如 {"app_group":"x"}）；可空、无索引。
	// 一等维度（租户/业务主体）走 company_id / subject_kind / subject_id 三列，不要再塞进这里。
	Labels string `gorm:"column:labels;type:text;comment:业务维度标签（JSON）"`
}

// TableName 固定表名。
func (CoreAsyncTask) TableName() string { return CoreAsyncTaskTableName }

// CoreAsyncTaskRun 是单次尝试记录，对应表 core_async_task_run。
//
// 一条任务有 0..N 次尝试，按 generation（重放轮次）分组；
// (async_task_id, generation, attempt) 唯一，表示同一轮次第 N 次尝试只允许一行。
type CoreAsyncTaskRun struct {
	ID string `gorm:"column:id;primaryKey;type:varchar(36);comment:尝试 ID（UUID v4）"`
	// created_at 单独索引：兜底清理按范围删除。
	CreatedAt time.Time `gorm:"column:created_at;index:idx_core_async_task_run_created_at;comment:创建时间"`

	AsyncTaskID string `gorm:"column:async_task_id;type:varchar(36);not null;uniqueIndex:uk_core_async_task_run_async_task_id_generation_attempt,priority:1;comment:所属任务 ID"`
	Generation  int32  `gorm:"column:generation;not null;default:1;uniqueIndex:uk_core_async_task_run_async_task_id_generation_attempt,priority:2;comment:所属重放轮次"`
	Attempt     int32  `gorm:"column:attempt;not null;uniqueIndex:uk_core_async_task_run_async_task_id_generation_attempt,priority:3;comment:本轮第几次尝试"`

	Status      string `gorm:"column:status;type:varchar(16);not null;index:idx_core_async_task_run_status_started_at,priority:1;comment:尝试状态"`
	FailureKind string `gorm:"column:failure_kind;type:varchar(16);not null;default:'';comment:失败分类"`
	WorkerID    string `gorm:"column:worker_id;type:varchar(64);not null;default:'';comment:执行该次尝试的执行者"`

	StartedAt  time.Time  `gorm:"column:started_at;not null;index:idx_core_async_task_run_status_started_at,priority:2;comment:开始时间"`
	FinishedAt *time.Time `gorm:"column:finished_at;comment:结束时间"`

	DurationMs int64  `gorm:"column:duration_ms;not null;default:0;comment:本次耗时毫秒"`
	Result     string `gorm:"column:result;type:text;comment:本次结果快照"`
	ErrorMsg   string `gorm:"column:error_msg;type:text;comment:本次错误"`
	RequestID  string `gorm:"column:request_id;type:varchar(64);not null;default:'';index:idx_core_async_task_run_request_id;comment:请求 ID"`
	TraceID    string `gorm:"column:trace_id;type:varchar(64);not null;default:'';comment:本次执行的 TraceID"`
}

// TableName 固定表名。
func (CoreAsyncTaskRun) TableName() string { return CoreAsyncTaskRunTableName }

// CoreAsyncTaskTypeTableName 任务类型开关表名。
const CoreAsyncTaskTypeTableName = "core_async_task_type"

// CoreAsyncTaskType 是任务类型开关（仅记录被显式 Disable 的类型；无行 = 启用，fail-open）。
//
// 与 core_async_task（任务实例）解耦：本表只承载「某类型当前是否允许被认领执行」这一运营开关。
type CoreAsyncTaskType struct {
	// ID 代理主键（与其他 asynctask 表一致，均为 string id）；业务唯一键是 task_type（唯一索引）。
	ID        string    `gorm:"column:id;primaryKey;type:varchar(36);comment:主键"`
	TaskType  string    `gorm:"column:task_type;type:varchar(128);not null;uniqueIndex:uk_core_async_task_type_task_type;comment:任务类型"`
	Enabled   bool      `gorm:"column:enabled;not null;default:true;comment:是否启用"`
	UpdatedAt time.Time `gorm:"column:updated_at;comment:更新时间"`
}

// TableName 固定表名。
func (CoreAsyncTaskType) TableName() string { return CoreAsyncTaskTypeTableName }
