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
	// ChainID 任务链 ID（空串=独立任务）。同一条业务链的多个 step 共享同一 ChainID。
	ChainID string
	// Step 步骤序号，从 0 开始；独立任务恒为 0。
	Step int
	// PrevStep 前序步骤序号；-1 表示无前序（首步/独立任务）。认领要求前序 step 已 succeeded。
	PrevStep    int
	Generation  int // 重放轮次，从 1 开始
	Attempt     int // 本轮已尝试次数
	MaxAttempts int
	Timeout     time.Duration
	NextRunAt   time.Time
	LeaseUntil  *time.Time
	WorkerID    string
	StartedAt   *time.Time
	FinishedAt  *time.Time
	Duration    time.Duration
	LastError   string
	Result      []byte
	// Labels 通用业务维度（如 app_group/company_id/uin），序列化为 JSON 落 labels 列。
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

// HasRemainingAttempts 表示本轮是否还有剩余重试次数（MaxAttempts<=0 视为不设上限）。
func (t *AsyncTask) HasRemainingAttempts() bool {
	if t.MaxAttempts <= 0 {
		return true
	}
	return t.Attempt < t.MaxAttempts
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
	Type        string        // 必填
	Payload     []byte        // 可选，上限 MaxPayloadBytes
	Priority    int           // 可选，默认 0
	MaxAttempts int           // 可选，见「默认值优先级」
	Timeout     time.Duration // 可选，见「默认值优先级」
	RunAt       time.Time     // 可选，绝对时刻（调用方时钟）；与 Delay 互斥
	Delay       time.Duration // 可选，相对延迟（DB 时钟）；与 RunAt 互斥
	UniqKey     string        // 可选，唯一性范围 = 该行存在期间（受 Retention 影响）
	BizType     string
	BizID       string
	// ChainID 可选：指定任务链；为空时独立任务（SubmitAfter 会自动继承上游 ChainID）。
	ChainID string
	// Step 可选：步骤序号，默认 0。
	Step int
	// Labels 可选：通用业务维度键值（如 app_group/company_id/uin），落 labels 列，可被 AsyncTaskCond 精确过滤。
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
	// Labels 按 label 精确匹配（AND 语义；空=不过滤）。
	Labels map[string]string
	// CreatedAfter / CreatedBefore 创建时间区间（nil=不限）。
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	// OrderBy 排序；白名单 created_at/updated_at/priority/status，asc/desc；默认 "created_at desc"。
	OrderBy string
}

// AsyncTaskRunCond 尝试列表查询条件（manager 使用；根包只定义结构）。
type AsyncTaskRunCond struct {
	Page, PageSize int
	AsyncTaskID    string
	Generation     int
	Status         Status
	Failure        FailureKind
}
