package worker

import (
	"context"
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// Handler 是唯一必须实现的方法。
type Handler interface {
	Execute(ctx context.Context, task *asynctask.AsyncTask) error
}

// HandlerFunc 便于「只写一个函数」的场景。
func HandlerFunc(fn func(ctx context.Context, t *asynctask.AsyncTask) error) Handler {
	return handlerFunc(fn)
}

type handlerFunc func(ctx context.Context, t *asynctask.AsyncTask) error

func (f handlerFunc) Execute(ctx context.Context, t *asynctask.AsyncTask) error { return f(ctx, t) }

// ResultHandler 是「带结果」的执行契约：一次调用同时给出结果快照与错误。
//
// 为什么需要它（而不是只靠 ResultProvider）：Handler 是**每类型单实例、被多个执行槽共享**的，
// 无参的 TakeResult() 在并发 >1 时无法判断「这是谁的结果」。ResultHandler 把结果绑定到本次
// 调用，天然并发安全，是多并发场景的推荐契约。
type ResultHandler interface {
	// ExecuteResult 执行任务并返回本次结果快照（建议 JSON）；错误语义与 Handler.Execute 一致。
	ExecuteResult(ctx context.Context, task *asynctask.AsyncTask) ([]byte, error)
}

// resultHandlerAdapter 让 ResultHandler 能通过 Register 注册：Execute 只保留错误，
// 结果由 worker 在 safeExecute 里按 ResultHandler 身份单独取（适配器自身也实现该接口）。
type resultHandlerAdapter struct{ ResultHandler }

func (a resultHandlerAdapter) Execute(ctx context.Context, t *asynctask.AsyncTask) error {
	_, err := a.ExecuteResult(ctx, t)
	return err
}

// ResultProvider 是可选接口：Execute 返回后，worker 从它取本次执行的「结果快照」
// （序列化后的业务结果），写进 core_async_task.result，供管理面查询与重放参考。
//
// 为什么是可选接口而不是改 Handler：Handler 是稳定契约，改签名会让所有既有实现编译失败；
// 而结果快照是「有则更好」的能力——不实现即保持原行为（result 为空）。
//
// 注意：只在 Execute 成功时被调用；失败/超时/失权时不取，避免把半成品当结果落库。
//
// 并发注意：该接口没有 task 参数，实现方无法判断「是谁的结果」；只在每类型并发=1
// （或有其它串行保证）时可用。并发 >1 请改用 ResultHandler。
type ResultProvider interface {
	// TakeResult 返回本次执行的结果快照（建议 JSON）；返回 nil/空表示没有结果。
	TakeResult() []byte
}

// Lifecycle 是可选钩子，实现几个算几个。
type Lifecycle interface {
	BeforeExecute(ctx context.Context, task *asynctask.AsyncTask) error
	OnRetry(ctx context.Context, task *asynctask.AsyncTask, cause error) error
	OnSuccess(ctx context.Context, task *asynctask.AsyncTask) error
	OnFailure(ctx context.Context, task *asynctask.AsyncTask, cause error) error
	OnCanceled(ctx context.Context, task *asynctask.AsyncTask) error
}

// 控制流哨兵。这两个是 worker 包内自己的哨兵（根包 asynctask 不定义同名符号），
// 由 Handler/Lifecycle 返回以控制执行流，故留在 worker 包。
var (
	// ErrSkip 不执行 Execute，任务直接 skipped（BeforeExecute 返回）。
	ErrSkip = errors.New("asynctask: skip this task")
	// ErrNoRetry 立即 failed，不消耗剩余次数（Execute 返回）。
	ErrNoRetry = errors.New("asynctask: do not retry")
)

// buildCallback 按 outcome 构造交回 Source 的 EnsureFunc；把本次失败原因注入闭包。
//
// 说明：「失败是否可重试」最终由 Source 判定（次数是否用尽、ErrNoRetry 等）。worker 只能
// 基于错误语义给出初判：ErrNoRetry / panic 视为不可重试（选 OnFailure），timeout /
// handler_error 视为可重试（选 OnRetry）。Source 在事务内、且仅在仍持有时调用该回调。
func (w *Worker) buildCallback(h Handler, task *asynctask.AsyncTask, outcome asynctask.Outcome, kind asynctask.FailureKind, cause error) asynctask.EnsureFunc {
	if outcome == asynctask.OutcomeSkipped {
		return nil
	}
	lc := asLifecycle(h)
	if lc == nil {
		return nil
	}

	switch outcome {
	case asynctask.OutcomeSucceeded:
		return func(ctx context.Context) error { return lc.OnSuccess(ctx, task) }
	case asynctask.OutcomeCanceled:
		return func(ctx context.Context) error { return lc.OnCanceled(ctx, task) }
	case asynctask.OutcomeFailed:
		// 失败能否重试：panic / ErrNoRetry / 次数用尽一律不可重试；其余选 OnRetry。
		// 最终是否重试仍由 Source 判定，这里只决定 worker 交给 Source 的默认回调。
		if kind == asynctask.FailurePanic || errors.Is(cause, ErrNoRetry) || !task.HasRemainingAttempts() {
			return func(ctx context.Context) error { return lc.OnFailure(ctx, task, cause) }
		}
		return func(ctx context.Context) error { return lc.OnRetry(ctx, task, cause) }
	}
	return nil
}

// asLifecycle 把 Handler 收敛成可选钩子：未实现 Lifecycle 时返回 nil。
func asLifecycle(h Handler) Lifecycle {
	if lc, ok := h.(Lifecycle); ok {
		return lc
	}
	return nil
}

// process 处理单条任务：BeforeExecute → Execute（带超时/续租/recover）→ 分类 → Save。
func (w *Worker) process(taskType string, task *asynctask.AsyncTask) {
	start := time.Now()
	if task.Type == "" {
		task.Type = taskType
	}
	h := w.handlerFor(taskType, task)
	if h == nil {
		// 未注册类型不该被认领到；防御性放租约，避免占着不放。
		w.logger.Errorw(w.baseCtx, "asynctask worker handler missing", "task_type", taskType, "async_task_id", task.ID)
		w.releaseTask(task)
		return
	}
	attempt := task.Attempt
	w.emit("started", task, attempt, 0, nil)

	lc := asLifecycle(h)

	// BeforeExecute 不在事务内；返回 ErrSkip 时写 skipped，无回调。
	if lc != nil {
		if err := lc.BeforeExecute(w.baseCtx, task); err != nil {
			if errors.Is(err, ErrSkip) {
				w.saveResult(task, asynctask.Result{
					Outcome:  asynctask.OutcomeSkipped,
					Failure:  asynctask.FailureSkipped,
					Duration: time.Since(start),
				}, nil)
				w.emit("skipped", task, attempt, time.Since(start), nil)
				return
			}
			// BeforeExecute 的其它错误按 handler_error 失败处理（按 outcome 走失败链路）。
			res := asynctask.Result{
				Outcome:  asynctask.OutcomeFailed,
				Failure:  asynctask.FailureHandler,
				Err:      err,
				Duration: time.Since(start),
			}
			w.saveResult(task, res, w.buildCallback(h, task, res.Outcome, res.Failure, err))
			w.emit("failed", task, attempt, time.Since(start), err)
			return
		}
	}

	// execCtx 是本次执行的取消源：超时、停机、续租失权都通过它传递。
	execCtx, cancelExec := context.WithCancel(w.baseCtx)
	defer cancelExec()

	var leaseLost atomic.Bool
	stopRenew := w.startRenew(execCtx, func() {
		leaseLost.Store(true)
		cancelExec()
	}, task)
	defer stopRenew()

	timeout := w.execTimeout(task)
	execCtx, cancelTimeout := context.WithTimeout(execCtx, timeout)

	resultPayload, err, panicked := safeExecute(execCtx, h, task)
	// 必须在 cancel 之前读：cancel 之后 Err() 恒为 Canceled，超时会被误判。
	execErr := execCtx.Err()
	cancelTimeout()
	cancelExec()

	res := asynctask.Result{
		Payload:  resultPayload,
		Duration: time.Since(start),
		Err:      err,
	}
	switch {
	case panicked:
		res.Outcome = asynctask.OutcomeFailed
		res.Failure = asynctask.FailurePanic
	case leaseLost.Load():
		res.Outcome = asynctask.OutcomeCanceled
		res.Failure = asynctask.FailureLeaseExpired
	case err == nil:
		res.Outcome = asynctask.OutcomeSucceeded
	case errors.Is(err, ErrNoRetry):
		res.Outcome = asynctask.OutcomeFailed
		res.Failure = asynctask.FailureHandler
	case execErr == context.DeadlineExceeded || errors.Is(err, context.DeadlineExceeded):
		res.Outcome = asynctask.OutcomeFailed
		res.Failure = asynctask.FailureTimeout
	case w.baseCtx.Err() != nil:
		// 执行期间被停机/调用方取消（超时已在上一支排除）。
		res.Outcome = asynctask.OutcomeCanceled
		res.Failure = asynctask.FailureCanceled
	default:
		res.Outcome = asynctask.OutcomeFailed
		res.Failure = asynctask.FailureHandler
	}

	// 失权：本次结果已无意义（接管者在跑），不写终态、也不跑业务钩子。
	if leaseLost.Load() {
		w.logger.Warnw(w.baseCtx, "asynctask worker task lost lease, result dropped",
			"task_type", task.Type, "async_task_id", task.ID, "attempt", attempt)
		w.emit("lease_lost", task, attempt, res.Duration, err)
		return
	}

	// 停机导致的取消：不写终态（否则吞掉剩余重试次数），放租约让别的实例重跑。
	if res.Outcome == asynctask.OutcomeCanceled && w.fetchStop.Load() {
		w.releaseTask(task)
		w.emit("released", task, attempt, res.Duration, err)
		return
	}

	w.saveResult(task, res, w.buildCallback(h, task, res.Outcome, res.Failure, err))

	switch res.Outcome {
	case asynctask.OutcomeSucceeded:
		w.emit("succeeded", task, attempt, res.Duration, nil)
	case asynctask.OutcomeCanceled:
		w.emit("canceled", task, attempt, res.Duration, err)
	default:
		w.emit("failed", task, attempt, res.Duration, err)
	}
}

// startRenew 在任务执行期间周期性续租，返回停止函数。
//
// 段内任一「连续失败达阈值」或「明确失权（ErrLeaseLost）」即回调 onLost，
// 由调用方取消执行 ctx。执行耗时短于一个续租周期时不会发起任何续租。
func (w *Worker) startRenew(execCtx context.Context, onLost func(), task *asynctask.AsyncTask) func() {
	interval := w.cfg.LeaseTTL / 3
	if interval <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		fails := 0
		for {
			select {
			case <-done:
				return
			case <-execCtx.Done():
				return
			case <-ticker.C:
				rctx, cancel := context.WithTimeout(context.WithoutCancel(execCtx), interval)
				err := w.src.Renew(rctx, task)
				cancel()
				if err == nil {
					fails = 0
					continue
				}
				if errors.Is(err, asynctask.ErrLeaseLost) {
					w.logger.Warnw(execCtx, "asynctask worker renew found lease lost",
						"task_type", task.Type, "async_task_id", task.ID)
					onLost()
					return
				}
				fails++
				w.logger.Warnw(execCtx, "asynctask worker renew failed",
					"task_type", task.Type, "async_task_id", task.ID,
					"consecutive_failures", fails, "error", err)
				if fails >= renewMaxConsecutiveFailures {
					onLost()
					return
				}
			}
		}
	}()
	return func() { close(done) }
}

// execTimeout 解析本次执行超时：任务显式 Timeout > HandlerOption > 默认 30s。
func (w *Worker) execTimeout(task *asynctask.AsyncTask) time.Duration {
	if task.Timeout > 0 {
		return task.Timeout
	}
	if e := w.entryFor(task.Type); e != nil && e.opts.timeout > 0 {
		return e.opts.timeout
	}
	return 30 * time.Second
}

// handlerFor 返回该任务类型的处理器（注册后不再变更，读取无需加锁）。
func (w *Worker) handlerFor(taskType string, task *asynctask.AsyncTask) Handler {
	if e := w.entryFor(taskType); e != nil {
		return e.handler
	}
	if task != nil {
		if e := w.entryFor(task.Type); e != nil {
			return e.handler
		}
	}
	return nil
}

func (w *Worker) entryFor(taskType string) *handlerEntry {
	if taskType == "" {
		return nil
	}
	return w.handlers[taskType]
}

// safeExecute 调 Execute，用 recover 捕获 panic；返回 (payload, err, panicked)。
func safeExecute(ctx context.Context, h Handler, task *asynctask.AsyncTask) (payload []byte, err error, panicked bool) {
	defer func() {
		if r := recover(); r != nil {
			stack := debug.Stack()
			top := topStackFrame(stack)
			err = fmt.Errorf("asynctask: handler panic: %v\n%s", r, top)
			panicked = true
		}
	}()
	// 优先「带结果」契约：结果与本次调用绑定，并发安全。
	if rh, ok := h.(ResultHandler); ok {
		payload, err = rh.ExecuteResult(ctx, task)
		return payload, err, false
	}
	err = h.Execute(ctx, task)
	return payloadOf(h, err), err, false
}

// payloadOf 只在执行成功时向 ResultProvider 取结果快照。
//
// 失败时刻意不取：半成品结果被当成功结果落库，比没有结果更难排查。
func payloadOf(h Handler, err error) []byte {
	if err != nil {
		return nil
	}
	if rp, ok := h.(ResultProvider); ok {
		return rp.TakeResult()
	}
	return nil
}

func topStackFrame(stack []byte) string {
	s := string(stack)
	lines := strings.Split(s, "\n")
	var out []string
	for _, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == "" || strings.HasPrefix(t, "goroutine ") || strings.HasPrefix(t, "panic: ") {
			continue
		}
		out = append(out, ln)
		if len(out) >= 3 {
			break
		}
	}
	if len(out) == 0 {
		return s
	}
	return strings.Join(out, "\n")
}

// saveResult 调 src.Save。ctx 与执行期上下文解耦（否则停机取消会让整个保存回滚），
// 但带独立超时，避免 Save 卡死拖住停机。
func (w *Worker) saveResult(task *asynctask.AsyncTask, res asynctask.Result, cb asynctask.EnsureFunc) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(w.baseCtx), w.cfg.ShutdownTimeout)
	defer cancel()
	if err := w.src.Save(ctx, task, res, cb); err != nil {
		if errors.Is(err, asynctask.ErrLeaseLost) {
			w.logger.Warnw(ctx, "asynctask worker save skipped: lease lost",
				"task_type", task.Type, "async_task_id", task.ID, "outcome", res.Outcome)
			return
		}
		w.logger.Errorw(ctx, "asynctask worker save error",
			"task_type", task.Type, "async_task_id", task.ID, "outcome", res.Outcome, "error", err)
	}
}

func (w *Worker) emit(typ string, task *asynctask.AsyncTask, attempt int, d time.Duration, err error) {
	if w.cfg.EventHandler == nil {
		return
	}
	w.cfg.EventHandler(Event{
		Type:        typ,
		AsyncTaskID: task.ID,
		TaskType:    task.Type,
		WorkerID:    w.cfg.WorkerID,
		Attempt:     attempt,
		Duration:    d,
		Err:         err,
	})
}
