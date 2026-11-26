package asynctask

import (
	"context"
	"time"
)

// Source 决定任务从哪来、结果往哪去。Worker 只依赖它。
//
// 本地实现（source/db）在事务内调用回调，TxFromContext 非 nil；
// 远程实现（source/remote）只是回调，TxFromContext 为 nil。
//
// 所有权契约：Fetch 成功即表示「本次执行持有该任务的租约」，直到 Save/Release 之一被调用。
//   - 执行期间应周期性 Renew（Worker 按 LeaseTTL/3 续租），续租失败返回 ErrLeaseLost，
//     调用方应停止执行并放弃 Save 的业务钩子；
//   - 主动放弃（停机、认领后无法交付）用 Release，让任务立即可被他人认领；
//   - Save 在「已失权」时必须返回 ErrLeaseLost 且不得执行 callback（副作用与状态不可分裂）。
type Source interface {
	// Fetch 取一条待执行任务。无任务时返回 ErrNoAsyncTask（实现可短暂阻塞）。
	Fetch(ctx context.Context, taskType string) (*AsyncTask, error)
	// Renew 续租所有权。返回 ErrLeaseLost 表示已失权（被接管/被取消/已终态），
	// 调用方应立即取消执行 ctx；其它错误视为可重试的瞬时故障。
	Renew(ctx context.Context, task *AsyncTask) error
	// Release 主动放弃所有权（停机或认领后无法交付时用），之后任务应尽快可被他人认领。
	// 语义上等价于「把租约置为已过期」；对不支持释放的实现可为空操作。
	Release(ctx context.Context, task *AsyncTask) error
	// Save 保存一次执行结果，并在合适的时机调用 callback
	// （本地实现在事务内，远程实现在回传前或回传后，由其决定）。
	// 结果如何流转（是否重试、下一次何时）由 Source 决定，不由 Worker 决定。
	// 已失权时返回 ErrLeaseLost，且不得调用 callback。
	Save(ctx context.Context, task *AsyncTask, r Result, callback EnsureFunc) error
	// Close 释放连接（本地关 Store 引用，远程关 HTTP 连接）。
	Close(ctx context.Context) error
}

// Outcome 单次尝试的结局分类。重试与否由 Source 判定。
type Outcome string

const (
	OutcomeSucceeded Outcome = "succeeded"
	OutcomeFailed    Outcome = "failed"
	OutcomeSkipped   Outcome = "skipped"
	OutcomeCanceled  Outcome = "canceled"
	// OutcomePaused 表示本次执行「主动让出」：不是失败、不进终态，任务结算后回 pending
	// 等待下一次执行（下一次认领会新建一条 attempt，即新的 run 行）。
	//
	// 本地 Source（source/db）在同一事务内按「核验租约 → 收尾 run（finishRun）→ 结算钩子 →
	// 任务回 pending」的顺序完成；钩子失败则整个事务回滚，任务仍 running，租约到期后由回收器
	// 按预留额结算。暂停**不消耗** FailureCount（失败重试额度），所以「连续暂停」不会被误判成
	// 「反复失败」。
	OutcomePaused Outcome = "paused"
)

// Result 是一次尝试的客观结果；重试与否由 Source 判定。
type Result struct {
	Outcome  Outcome // Succeeded / Failed / Skipped / Canceled / Paused
	Failure  FailureKind
	Err      error
	Payload  []byte // handler 返回的结果快照
	Duration time.Duration
	// PauseUntil 是 Outcome==OutcomePaused 时的「最早可再次认领时间」；
	// 零值表示立即可被认领（让出资源但不等待）。
	PauseUntil time.Time
	// PauseReason 是 Outcome==OutcomePaused 时的让出原因，落到任务行的 last_error 便于排障
	// （暂停不是错误，这里只是复用一列写「为什么让出」）。
	PauseReason string
}

// EnsureFunc 是业务钩子；Source 决定在哪调用它。
type EnsureFunc func(ctx context.Context) error

// StatusReader 是 Source 的**可选**扩展：只读任务当前状态。
//
// 用途：取消通知（见 CancelWatcher）可能漏、也可能与执行登记撞车，执行侧需要一条
// 「直接查库对账」的通路来兜底；它也是「定时补查」的实现基础。
// 本地 Source（source/db）实现了它；装饰/远程 Source 需要显式转发才有该能力。
type StatusReader interface {
	TaskStatus(ctx context.Context, id string) (Status, error)
}

// LeaseTxn 是 Source 的**可选**扩展：在事务内锁住任务行、校验仍持有租约，再执行 fn。
//
// 为什么需要它（设计 §9.2「持租约事务接口或事务钩子」）：下游要把自己的一张表（如阶段预算 /
// 断点产物）与「仍持有租约」这件事放在**同一个事务**里结算；Save 的钩子只覆盖「一次执行的收尾」，
// 而预留额申请、断点发布这些动作发生在执行中间，需要一个通用的持租约事务入口。
//
// 契约：
//   - 已失权（被接管 / 被取消 / 已终态 / generation|attempt 不匹配）返回 ErrLeaseLost，且**不调用** fn；
//   - 持有租约时以 ctx 调用 fn，此时 TxFromContext(ctx) 非 nil（动态类型 *gorm.DB），
//     fn 的错误会让整个事务回滚；
//   - 远程 Source 无法提供本地事务语义，不实现本接口（断言失败即按「不支持」处理）。
type LeaseTxn interface {
	WithLeaseTx(ctx context.Context, task *AsyncTask, fn EnsureFunc) error
}

// RunReclaimHook 是回收器（reaper）在关闭一次「失权执行」时调用的钩子（同一事务内）。
//
// 为什么需要它（设计 §9.2「失权回收也结清预留」）：执行进程崩溃后，它申请的阶段预算预留额
// 只能由回收器结算——按**预留额**扣费而不是「没扣」（否则重启可无限刷新预算）。
//
// 参数：ctx 带事务（TxFromContext 非 nil）；run 是被回收的尝试行快照（含 generation/attempt，
// 可唯一定位这次执行）；reason 是回收原因（当前为 FailureLeaseExpired）。
//
// 实现必须幂等：钩子返回错误会让本次回收回滚并在下一轮重试，届时同一 run 会再次进入钩子。
type RunReclaimHook func(ctx context.Context, run *AsyncTaskRun, reason FailureKind) error

// txContextKey 是 TxFromContext 在 ctx 中存取事务句柄的私有键。
type txContextKey struct{}

// WithTx 把事务句柄注入 ctx，供 TxFromContext 取用。
// 仅供本地 Source 在「事务内调用钩子」时使用；业务侧一般不直接调用。
func WithTx(ctx context.Context, tx any) context.Context {
	return context.WithValue(ctx, txContextKey{}, tx)
}

// TxFromContext 仅在「本地 Source 在事务内调用回调」时非 nil。
// 远程 Source 下返回 nil——不要在钩子里假设一定拿到事务。
//
// 返回类型是 gorm-free 的抽象句柄（any），以保证根包与 worker 的依赖树不含 gorm。
// 本地形态下它的动态类型是 *gorm.DB，业务侧按需类型断言：
//
//	if db, ok := asynctask.TxFromContext(ctx).(*gorm.DB); ok { _ = db.Model(...).Update(...) }
func TxFromContext(ctx context.Context) any {
	if ctx == nil {
		return nil
	}
	return ctx.Value(txContextKey{})
}
