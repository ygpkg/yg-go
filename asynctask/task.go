// Package asynctask 是任务中心的契约内核：实体（AsyncTask / AsyncTaskRun）、任务规格（AsyncTaskSpec）、
// 查询条件（AsyncTaskCond / AsyncTaskRunCond）、状态与失败分类、Source 取任务接口、错误哨兵。
//
// 依赖方向：manager（任务中心）与 worker（执行运行时）互不依赖，只在根包碰面；
// worker 不引入 gorm。由 check-asynctask-deps.sh 的 go list -deps 断言。
//
// 本包移植自 golib 的 task/gasync（MIT）；通用能力（日志/建表等）改用 yg-go 自身的包。
package asynctask

import (
	"encoding/json"
	"time"
)

// Status 任务状态。
type Status string

const (
	StatusPending   Status = "pending"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusFailed    Status = "failed"
	StatusCanceled  Status = "canceled"
	StatusSkipped   Status = "skipped" // BeforeExecute 返回 ErrSkip
	StatusTimeout   Status = "timeout" // 执行超时（独立状态；兼容旧的 failed+failure_kind=timeout）
	// StatusPaused 是**只在 core_async_task_run 上出现**的状态：本次执行主动让出（结算完成、
	// 任务已回 pending），本次尝试不是失败、也不消耗失败重试额度。
	//
	// 任务实体（core_async_task）永远不会取这个值：暂停后它的状态是 pending。
	StatusPaused Status = "paused"
)

// FailureKind 与 Status 正交：状态说「结束成什么样」，分类说「为什么」。
type FailureKind string

const (
	FailureNone         FailureKind = ""
	FailureHandler      FailureKind = "handler_error"
	FailureTimeout      FailureKind = "timeout"
	FailurePanic        FailureKind = "panic"
	FailureCanceled     FailureKind = "canceled"
	FailureLeaseExpired FailureKind = "lease_expired"
	FailureSkipped      FailureKind = "skipped"
	// FailurePaused 是「主动让出」的分类（配合 StatusPaused 使用）。
	FailurePaused FailureKind = "paused"
)

// AsyncTask 是一条有状态的任务实体，对应表 core_async_task。
type AsyncTask struct {
	ID       string
	Type     string
	Status   Status
	Priority int
	Payload  []byte
	UniqKey  string // 空串表示无幂等键
	BizType  string
	BizID    string
	// CompanyID 租户 ID（0 = 系统/无租户）。一等维度，走独立列与索引，不进 Labels。
	CompanyID uint64
	// SubjectKind / SubjectID 业务主体（如 work/<uuid>）：通用「按主体取最近一条/列表」的维度。
	SubjectKind string
	SubjectID   string
	// ChainID 任务链 ID（空串=独立任务）。同一条业务链的多个 step 共享同一 ChainID。
	ChainID string
	// Step 步骤序号，从 0 开始；独立任务恒为 0。
	Step int
	// PrevStep 前序步骤序号；-1 表示无前序（首步/独立任务）。认领要求前序 step 已 succeeded。
	PrevStep   int
	Generation int // 重放轮次，从 1 开始
	// Attempt 是本轮**单调递增的执行序号**：每次认领 +1，一次执行对应一行
	// core_async_task_run（(id, generation, attempt) 唯一），也是租约隔离与「晚到结果/晚到取消
	// 认不认得出自己」的定位依据。
	//
	// 它**不是**失败重试计数：主动暂停也会 +1（因为暂停同样是一次独立的 run），
	// 但不会消耗重试额度。判「还能不能让任务再跑」请看 FailureCount / MaxAttempts。
	Attempt int
	// FailureCount 是本轮的可重试失败次数（只在「可重试失败 / 失权回收」时 +1；暂停不加）。
	//
	// 准入条件是 FailureCount < MaxAttempts：这样暂停（让出）不会被算作失败，
	// 而真正失败仍受 MaxAttempts 约束。存量行由升级片段回填成 attempt 以保持旧口径。
	FailureCount int
	MaxAttempts  int
	Timeout      time.Duration
	NextRunAt    time.Time
	LeaseUntil   *time.Time
	WorkerID     string
	StartedAt    *time.Time
	FinishedAt   *time.Time
	Duration     time.Duration
	LastError    string
	Result       []byte
	// Labels 开放业务维度（如 app_group/uin），序列化为 JSON 落 labels 列（无索引）。
	// 租户与业务主体已是一等维度（CompanyID / SubjectKind / SubjectID），不要塞进 Labels。
	Labels    map[string]string
	RequestID string
	TraceID   string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Bind 反序列化 Payload 到 v。
//
// 空 payload 直接返回 nil（无入参的任务不必特殊处理）：交给 json.Unmarshal 会得到
// "unexpected end of JSON input"，而被当作参数非法 → 不重试 → 任务直接 failed。
func (t *AsyncTask) Bind(v any) error {
	if t == nil || len(t.Payload) == 0 {
		return nil
	}
	return json.Unmarshal(t.Payload, v)
}

// HasRemainingAttempts 表示「本次失败之后」本轮是否还能再重试（MaxAttempts<=0 视为不设上限）。
//
// 判据是 FailureCount 而不是 Attempt：暂停后的下一次执行不再消耗失败额度，
// 所以 attempt 会大于 max_attempts 而你仍可能持有剩余重试额度。
// 实现与 Source 的「最后一次失败直接终结」判定一致（见 source/db 的 Save）：
// 本次是第 FailureCount+1 次失败，只有它仍小于 MaxAttempts 才允许回退重试。
func (t *AsyncTask) HasRemainingAttempts() bool {
	if t.MaxAttempts <= 0 {
		return true
	}
	return t.FailureCount+1 < t.MaxAttempts
}

// Claimable 表示本任务当前是否还满足「可被认领」的失败额度条件（FailureCount < MaxAttempts）。
//
// 与认领 SQL 的谓词同口径（见 source/db 的 claimSelectAndFlip）。暂停不改变 FailureCount，
// 因此暂停后的任务仍是 Claimable=true。
func (t *AsyncTask) Claimable() bool {
	if t.MaxAttempts <= 0 {
		return true
	}
	return t.FailureCount < t.MaxAttempts
}

// IsStuck 判断任务是否卡住：running 且租约已过期（now 之后无可续租的归属）。
func (t *AsyncTask) IsStuck(now time.Time) bool {
	if t.Status != StatusRunning || t.LeaseUntil == nil {
		return false
	}
	return t.LeaseUntil.Before(now)
}

// CanCancel 判断任务是否可取消：pending 或 running。
func (t *AsyncTask) CanCancel() bool {
	return t.Status == StatusPending || t.Status == StatusRunning
}

// AsyncTaskRun 是一次尝试记录，对应表 core_async_task_run。一条任务有 0..N 次尝试，按 generation 分组。
type AsyncTaskRun struct {
	ID          string
	AsyncTaskID string
	Generation  int
	Attempt     int
	Status      Status
	Failure     FailureKind
	WorkerID    string
	StartedAt   time.Time
	FinishedAt  *time.Time
	Duration    time.Duration
	Result      []byte
	Error       string
	RequestID   string
	TraceID     string
	CreatedAt   time.Time
}

// AsyncTaskSpec 是提交任务时的参数。显式值落库非 0，其余在 Fetch 时按「AsyncTaskSpec > HandlerOption > 配置默认」解析。
type AsyncTaskSpec struct {
	Type     string // 必填
	Payload  []byte // 可选，上限 MaxPayloadBytes
	Priority int    // 可选，默认 0
	// MaxAttempts 是**可重试失败次数的上限**（不是执行次数上限）：暂停（主动让出）不计入。
	// <=0（未显式指定）表示用配置默认值 DefaultMaxAttempts（默认 3，配置校验要求它 >0）；
	// 平台侧不会出现 max_attempts = 0 的行。
	MaxAttempts int           // 可选，见「默认值优先级」
	Timeout     time.Duration // 可选，见「默认值优先级」
	RunAt       time.Time     // 可选，绝对时刻（调用方时钟）；与 Delay 互斥
	Delay       time.Duration // 可选，相对延迟（DB 时钟）；与 RunAt 互斥
	UniqKey     string        // 可选，唯一性范围 = 活跃期（pending/running）；终态后同键可重建
	BizType     string
	BizID       string
	// CompanyID 可选：租户 ID（0 = 系统/无租户）。
	CompanyID uint64
	// SubjectKind / SubjectID 可选：业务主体类型与 ID（字符串化，如 "work" / "<uuid>"）。
	SubjectKind string
	SubjectID   string
	// ChainID 可选：指定任务链；为空时独立任务（SubmitAfter 会自动继承上游 ChainID）。
	ChainID string
	// Step 可选：步骤序号，默认 0。
	Step int
	// Labels 可选：开放业务维度键值（如 app_group/uin），落 labels 列，可被 AsyncTaskCond 精确过滤。
	Labels    map[string]string
	RequestID string // 可选，覆盖从 ctx 取的 request id
}

// AsyncTaskCond 任务列表查询条件（manager 使用；根包只定义结构）。
type AsyncTaskCond struct {
	Page, PageSize int
	Type           string
	Status         Status
	BizType        string
	BizID          string
	// ChainID 按任务链过滤（空=不过滤）。
	ChainID string
	// CompanyID 按租户过滤（nil=不过滤；0 是合法租户，不能用零值判断）。
	CompanyID *uint64
	// SubjectKind / SubjectID 按业务主体过滤（空=不过滤）。
	SubjectKind string
	SubjectID   string
	// UniqKey 按幂等键过滤（空=不过滤）。
	UniqKey string
	// ActiveOnly 只返回活跃期（pending/running）的任务。
	ActiveOnly bool
	// Labels 按 label 精确匹配（AND 语义；空=不过滤）。
	Labels map[string]string
	// CreatedAfter / CreatedBefore 创建时间区间（nil=不限）。
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	// OrderBy 排序；白名单 created_at/updated_at/priority/status，asc/desc；默认 "created_at desc"。
	OrderBy string
	// AfterCreatedAt / AfterID keyset 分页游标：只返回排在游标之后的行，
	// 排序方向跟随 OrderBy（默认 created_at desc）。与 Page 并存，深页请优先用它。
	AfterCreatedAt *time.Time
	AfterID        string
}

// AsyncTaskRunCond 尝试列表查询条件（manager 使用；根包只定义结构）。
type AsyncTaskRunCond struct {
	Page, PageSize int
	AsyncTaskID    string
	Generation     int
	Status         Status
	Failure        FailureKind
}
