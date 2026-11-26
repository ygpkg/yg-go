package db

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/gorm"
)

// DefaultMinPauseInterval 是「暂停（主动让出）」恢复的最短间隔。
//
// 为什么需要它：暂停不消耗失败额度、也不限次数，而 handler 返回 worker.Pause(0, ...) 的语义是
// 「不额外等待，把算力让出去」。若 next_run_at 直接取 now，任务会按 PollInterval（默认 500ms）
// 被反复认领，每次产出一条 core_async_task_run。这类任务永不进入终态，而 Cleanup 只删终态任务
// 的尝试行，于是 run 表被只增不减地灌满，且没有任何失败告警。1s 的下限既保留「让出后马上能再跑」，
// 又不会把 run 表打爆。
const DefaultMinPauseInterval = time.Second

// gormDB 是 *gorm.DB 的别名，便于在 source.go 内以短名引用。
type gormDB = gorm.DB

// Source 是 manager 的本地 Source 适配器：把 DB 任务中心适配成 worker 的 asynctask.Source。
//
// 本地形态在事务内调用回调（asynctask.TxFromContext 非 nil），并把任务终态、尝试行收尾
// 与重试退避写在同一事务里。是否重试由 Source（这里）决定，不由 Worker 决定。
//
// 所有权语义：Save 先按 (worker_id, generation, attempt) 锁住任务行确认仍持有租约，
// 再跑业务钩子——失权时不跑钩子、返回 asynctask.ErrLeaseLost，保证副作用与状态不分裂。
type source struct {
	store    *store
	db       *gorm.DB
	dialect  Dialect
	backoff  asynctask.Backoff
	workerID string
	// failFastChain 为 true 时，某 step 失败终态后同 chain 后续 step 置 canceled。
	failFastChain bool
	// maxResultBytes 是结果快照的截断上限（来自 Config.MaxResultBytes，默认 4096）。
	maxResultBytes int
}

// 编译期断言：source 满足 asynctask.Source，并实现两个可选扩展接口
// （取消兜底对账用 StatusReader；持租约事务给下游用 LeaseTxn）。
var (
	_ asynctask.Source       = (*source)(nil)
	_ asynctask.StatusReader = (*source)(nil)
	_ asynctask.LeaseTxn     = (*source)(nil)
)

// Fetch 以本 Source 的身份认领一条任务。无候选返回 asynctask.ErrNoAsyncTask。
//
// 认领事务内会一并写入本次尝试的 running 行，因此这里拿到的就是「已翻转为 running、
// attempt 已 +1、worker_id 已写入」的完整任务，无需二次查询。
func (s *source) Fetch(ctx context.Context, taskType string) (*asynctask.AsyncTask, error) {
	return s.ClaimFor(ctx, taskType, s.workerID)
}

// ClaimFor 以指定执行者身份认领一条任务。
//
// 供「远程任务中心」代表远端 worker 认领：中心侧的 Source 只有一个固定 workerID，
// 不能直接用它去认领远端 worker 的任务，否则续租/回传的归属校验全都会错位。
// workerID 为空返回 ErrInvalidConfig；无候选返回 asynctask.ErrNoAsyncTask。
func (s *source) ClaimFor(ctx context.Context, taskType, workerID string) (*asynctask.AsyncTask, error) {
	if strings.TrimSpace(workerID) == "" {
		return nil, fmt.Errorf("%w: worker_id is empty", asynctask.ErrInvalidConfig)
	}
	rows, err := s.store.claimBatch(ctx, taskType, workerID, 1)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, asynctask.ErrNoAsyncTask
	}
	return asyncTaskRowToDomain(rows[0]), nil
}

// Renew 续租。已失权（被接管/被取消/已终态）时返回 asynctask.ErrLeaseLost，调用方应停止执行。
func (s *source) Renew(ctx context.Context, task *asynctask.AsyncTask) error {
	if task == nil || task.ID == "" {
		return nil
	}
	res, err := s.store.renew(ctx, task.ID, task.WorkerID)
	if err != nil {
		return err
	}
	switch res {
	case RenewCanceled, RenewLeaseExpired:
		return asynctask.ErrLeaseLost
	default:
		return nil
	}
}

// Release 主动放弃所有权：租约置为「已过期」并清空 worker_id。
//
// 清 worker_id 是必要的：被放弃的那次执行若稍后才返回，Save 的所有权校验会命中 0 行
// 而返回 ErrLeaseLost，从而不会执行钩子。
func (s *source) Release(ctx context.Context, task *asynctask.AsyncTask) error {
	if task == nil || task.ID == "" {
		return nil
	}
	sql := fmt.Sprintf(`UPDATE %s SET lease_until = %s, worker_id = ''
WHERE id = ? AND worker_id = ? AND status = 'running'`,
		CoreAsyncTaskTableName, s.dialect.nowValueExpr())
	return s.db.WithContext(ctx).Exec(sql, task.ID, task.WorkerID).Error
}

// Reap 运行一次回收（① 收口残留尝试行、② 判死次数用尽），返回处理的行数。
// 供 Manager 的后台维护循环调用；单次 batch<=0 时取 200。
func (s *source) Reap(ctx context.Context, batch int) (runs int64, asyncTasks int64, err error) {
	return s.store.reap(ctx, batch)
}

// Close 释放连接（本地形态无长连接，空操作）。
func (s *source) Close(ctx context.Context) error { return nil }

// Save 保存一次执行结果，并在事务内调用回调。
//
// 执行顺序（顺序本身就是契约）：
//  1. 锁住任务行，确认仍是本 worker、同一 generation/attempt 在跑；
//  2. 收尾本次尝试行，并记住「这次 run 是不是由本事务关闭的」（失败计数只在是的这次 +1）；
//  3. 按结局推进任务状态，并在同一事务内调用业务钩子。
//
// 失权时不执行钩子、返回 asynctask.ErrLeaseLost：此时写终态会被守卫拦下，
// 钩子里的副作用却无法回滚，所以「不跑」比「跑了不记」正确。
//
// 结局分支（设计 §9.3）：
//   - succeeded / canceled / skipped：终态，不动 failure_count；
//   - paused：**结算后回 pending**，不动 failure_count、动 attempt（下一次认领 +1，产生新的 run 行）；
//   - failed：failure_count+1；还有额度就退避重试，额度用尽则终结（timeout 落 StatusTimeout）。
func (s *source) Save(ctx context.Context, task *asynctask.AsyncTask, r asynctask.Result, callback asynctask.EnsureFunc) error {
	if task == nil {
		return nil
	}
	now := s.dialect.nowValueExpr()

	return s.db.WithContext(ctx).Transaction(func(tx *gormDB) error {
		owned, err := s.lockOwnership(tx, task)
		if err != nil {
			return err
		}
		if !owned {
			return asynctask.ErrLeaseLost
		}
		// 事务句柄注入 ctx，供 callback 取用。
		cctx := asynctask.WithTx(ctx, tx)

		// 尝试行收尾（认领时已建行，这里只更新；缺失时补插，兼容历史行）。
		// closed=false 表示「别人（回收器）已经关闭了这次 run」——此时失败额度已经由对方扣过，
		// 本事务不能再扣一次（reaper 与正常结算同时到达只扣一次）。
		closed, err := s.finishRun(tx, task, r, now)
		if err != nil {
			return err
		}

		switch r.Outcome {
		case asynctask.OutcomeSucceeded:
			if callback != nil {
				if err := callback(cctx); err != nil {
					return err
				}
			}
			return s.finishAsyncTask(tx, task, asynctask.StatusSucceeded, r, now, 0)

		case asynctask.OutcomeCanceled:
			if callback != nil {
				_ = callback(cctx)
			}
			return s.finishAsyncTask(tx, task, asynctask.StatusCanceled, r, now, 0)

		case asynctask.OutcomeSkipped:
			return s.finishAsyncTask(tx, task, asynctask.StatusSkipped, r, now, 0)

		case asynctask.OutcomePaused:
			// 暂停结算：租约已在上方核验、run 已收尾，这里先在同一事务里跑业务钩子
			// （下游据此按「实际耗时」结算阶段预留额），再把任务放回 pending。
			// 钩子失败 → 整个事务回滚 → 任务仍 running，租约到期后由回收器按预留额结算。
			if callback != nil {
				if err := callback(cctx); err != nil {
					return err
				}
			}
			return s.pauseAsyncTask(tx, task, r)

		default: // Failed
			if callback != nil {
				if err := callback(cctx); err != nil {
					return err
				}
			}
			// 是否重试，按「本次失败记账之后」的额度判：
			//   closed=true  → 本次失败还没进 failure_count（本事务里 +1），判 fc+1 < max；
			//   closed=false → 回收器已经关闭这次 run 并记过账（本次不再 +1），判 fc < max。
			// 两者差的就是这 1：写错会少给一次重试机会，或把任务留在永远领不到的 pending。
			// task.FailureCount 已由 lockOwnership 在行锁内刷新，不是认领时的快照。
			fits := task.Claimable()
			if closed {
				fits = task.HasRemainingAttempts()
			}
			if fits {
				return s.retryAsyncTask(tx, task, r, closed)
			}
			// 终态状态：超时归为 StatusTimeout（独立状态），其余失败为 StatusFailed。
			termStatus := asynctask.StatusFailed
			if r.Failure == asynctask.FailureTimeout {
				termStatus = asynctask.StatusTimeout
			}
			if err := s.finishAsyncTask(tx, task, termStatus, r, now, failureDelta(closed)); err != nil {
				return err
			}
			// 失败级联：任务链某 step 失败后，同 chain 后续 step 置 canceled，
			// 避免其因前序永不 succeeded 而永久 pending 积压（仅 FailFastChain 开启时）。
			return s.cancelDownstream(tx, task, now)
		}
	})
}

// failureDelta 把「本次 run 是否由本事务关闭」换算成 failure_count 的增量。
func failureDelta(closed bool) int {
	if closed {
		return 1
	}
	return 0
}

// WithLeaseTx 在事务内锁住任务行、校验仍持有租约，再执行 fn（实现 asynctask.LeaseTxn）。
//
// 用途：下游把「只在自己还持有租约时才允许做」的写操作（申请阶段预算预留额、发布断点产物）
// 与平台侧的租约状态放进同一个事务；fn 里用 asynctask.TxFromContext(ctx) 取事务句柄。
//
// 失权（被接管/被取消/已终态/generation|attempt 不匹配）返回 asynctask.ErrLeaseLost 且不调用 fn。
func (s *source) WithLeaseTx(ctx context.Context, task *asynctask.AsyncTask, fn asynctask.EnsureFunc) error {
	if task == nil || fn == nil {
		return nil
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gormDB) error {
		owned, err := s.lockOwnership(tx, task)
		if err != nil {
			return err
		}
		if !owned {
			return asynctask.ErrLeaseLost
		}
		return fn(asynctask.WithTx(ctx, tx))
	})
}

// TaskStatus 只读任务当前状态（实现 asynctask.StatusReader）。
//
// 取消通知可能漏、也可能与执行登记撞车，执行侧用它对账活跃执行；任务不存在返回
// asynctask.ErrAsyncTaskNotFound（调用方按「不必取消」处理即可）。
func (s *source) TaskStatus(ctx context.Context, id string) (asynctask.Status, error) {
	row, err := s.store.loadAsyncTaskRow(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", asynctask.ErrAsyncTaskNotFound
	}
	if err != nil {
		return "", err
	}
	return asynctask.Status(row.Status), nil
}

// lockOwnership 锁住任务行并判断本次执行是否仍持有租约。
//
// 条件同时校验 worker_id / status / generation / attempt：只看 worker_id 时，
// 同一进程（共用 worker_id）的陈旧 goroutine 会把别人重认领后的任务当成自己的。
//
// 命中时把库里的失败额度回填到 task（failure_count / max_attempts）：task 上的值是**认领时**
// 的快照，而回收器会在事务外给更早那次崩溃的执行补账（failure_count + 1），两者可能不一致。
// 「本次失败之后还有没有额度」必须用行锁内读到的值判，否则会把任务算成「还能重试」而实际
// 额度已空——那样的任务回 pending 后永远认领不到（认领谓词是 failure_count < max_attempts），
// 也永远不会被判死，等于静默丢任务。
func (s *source) lockOwnership(tx *gormDB, task *asynctask.AsyncTask) (bool, error) {
	sql := fmt.Sprintf(`SELECT id, failure_count, max_attempts FROM %s
WHERE id = ? AND worker_id = ? AND status = 'running' AND generation = ? AND attempt = ?%s`,
		CoreAsyncTaskTableName, s.dialect.rowLockClause())
	var row struct {
		ID           string
		FailureCount int32
		MaxAttempts  int32
	}
	if err := tx.Raw(sql, task.ID, task.WorkerID, task.Generation, task.Attempt).Scan(&row).Error; err != nil {
		return false, err
	}
	if row.ID == "" {
		return false, nil
	}
	task.FailureCount = int(row.FailureCount)
	task.MaxAttempts = int(row.MaxAttempts)
	return true, nil
}

// cancelDownstream 把同 chain 中 step 大于当前 step 的 pending 步骤置 canceled。
// 仅在 FailFastChain 开启时生效。chain_id 先单独查出来再用参数更新：
// MySQL 不允许「更新一张表的同时在子查询里读它」（错误 1093）。
func (s *source) cancelDownstream(tx *gormDB, task *asynctask.AsyncTask, nowExpr string) error {
	if !s.failFastChain {
		return nil
	}
	var chainID string
	if err := tx.Raw(fmt.Sprintf(`SELECT chain_id FROM %s WHERE id = ?`, CoreAsyncTaskTableName), task.ID).
		Scan(&chainID).Error; err != nil {
		return err
	}
	if chainID == "" {
		return nil
	}
	sql := fmt.Sprintf(`UPDATE %s
SET status = 'canceled', finished_at = %s
WHERE chain_id = ? AND step > ? AND status = 'pending'`,
		CoreAsyncTaskTableName, nowExpr)
	return tx.Exec(sql, chainID, task.Step).Error
}

// finishAsyncTask 写任务终态（succeeded/failed/canceled/skipped/timeout），清 lease/worker。
//
// failureDelta 只有「终结一次失败」时不为 0：终态失败同样消耗一次失败额度，
// 让 core_async_task.failure_count 始终等于「本轮真实可重试失败次数」。
func (s *source) finishAsyncTask(tx *gormDB, task *asynctask.AsyncTask, status asynctask.Status, r asynctask.Result, nowExpr string, failureDelta int) error {
	errMsg := ""
	if r.Err != nil {
		errMsg = truncate(r.Err.Error(), 1024)
	}
	sql := fmt.Sprintf(`UPDATE %s
SET status = ?, finished_at = %s, lease_until = NULL, worker_id = '',
    duration_ms = duration_ms + ?, last_error = ?, result = ?,
    failure_count = failure_count + ?
WHERE id = ? AND worker_id = ? AND status = 'running' AND generation = ? AND attempt = ?`,
		CoreAsyncTaskTableName, nowExpr)
	res := tx.Exec(sql, string(status), r.Duration.Milliseconds(), errMsg, s.resultText(r.Payload), failureDelta,
		task.ID, task.WorkerID, task.Generation, task.Attempt)
	if res.Error != nil {
		return res.Error
	}
	// 所有权已在上方 lockOwnership 锁定，理论上一定命中；命中 0 行说明并发语义被破坏。
	if res.RowsAffected == 0 {
		return asynctask.ErrLeaseLost
	}
	task.Status = status
	return nil
}

// retryAsyncTask 可重试失败：回 pending，next_run_at=now+backoff（DB 表达式）。
//
// failure_count 只在本次 run 由本事务关闭时才 +1（closed）；退避按失败次数而不是 attempt 取
// ——暂停会让 attempt 大于失败次数，用 attempt 取退避会把退避时长算得过长。
func (s *source) retryAsyncTask(tx *gormDB, task *asynctask.AsyncTask, r asynctask.Result, closed bool) error {
	backoff := s.backoff(task.FailureCount + 1)
	errMsg := ""
	if r.Err != nil {
		errMsg = truncate(r.Err.Error(), 1024)
	}
	sql := fmt.Sprintf(`UPDATE %s
SET status = 'pending', lease_until = NULL, worker_id = '',
    next_run_at = %s, duration_ms = duration_ms + ?, last_error = ?,
    failure_count = failure_count + ?
WHERE id = ? AND worker_id = ? AND status = 'running' AND generation = ? AND attempt = ?`,
		CoreAsyncTaskTableName, s.dialect.afterExpr())
	res := tx.Exec(sql, s.dialect.afterArg(backoff), r.Duration.Milliseconds(), errMsg, failureDelta(closed),
		task.ID, task.WorkerID, task.Generation, task.Attempt)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return asynctask.ErrLeaseLost
	}
	task.Status = asynctask.StatusPending
	if closed {
		task.FailureCount++
	}
	return nil
}

// pauseAsyncTask 暂停结算的最后一步：任务回 pending（租约、worker、run 都已在同一事务内处理）。
//
// 关键差异（设计 §9.3 Y3/Y5）：
//   - status 回 pending 而不是终态：暂停是「让出」，不是结束；
//   - failure_count **不动**：暂停不消耗失败重试额度，否则连续暂停会被误判成连续失败；
//   - attempt **不归零**：它继续做单调执行序号，下一次认领 +1，产生一条新的 run 行，
//     这样「谁在哪次执行里做了什么」始终可追（core_async_task_run 全量留痕）；
//   - next_run_at 取 Result.PauseUntil；零值（handler 只要求「让出」、不指定恢复时刻）时退化成
//     now + DefaultMinPauseInterval，而不是 now——否则暂停会变成忙等，见该常量的说明；
//   - duration 继续累计。
func (s *source) pauseAsyncTask(tx *gormDB, task *asynctask.AsyncTask, r asynctask.Result) error {
	reason := r.PauseReason
	if reason == "" {
		reason = "paused"
	}
	nextExpr, nextArg := s.dialect.afterExpr(), s.dialect.afterArg(DefaultMinPauseInterval)
	if !r.PauseUntil.IsZero() {
		nextExpr, nextArg = "?", r.PauseUntil
	}
	sql := fmt.Sprintf(`UPDATE %s
SET status = 'pending', lease_until = NULL, worker_id = '',
    next_run_at = %s, duration_ms = duration_ms + ?, last_error = ?
WHERE id = ? AND worker_id = ? AND status = 'running' AND generation = ? AND attempt = ?`,
		CoreAsyncTaskTableName, nextExpr)
	args := []any{nextArg, r.Duration.Milliseconds(), truncate(reason, 1024),
		task.ID, task.WorkerID, task.Generation, task.Attempt}
	res := tx.Exec(sql, args...)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return asynctask.ErrLeaseLost
	}
	task.Status = asynctask.StatusPending
	task.LastError = reason
	return nil
}

// finishRun 收尾本次尝试行：认领时已插入 running 行，这里 UPDATE 成终态。
//
// 返回 closed：本次 run 是否由**本事务**从 running 关闭。
// 失败额度与失权预留额的扣减只挂在 closed=true 上——回收器用同一个守卫
// （UPDATE ... AND status='running'），所以两者同时到达时只有一方扣费。
//
// 行不存在（历史数据、或 Source 不是本包写入的认领行）时补插一条，
// started_at 用 DB 表达式「now - 本次耗时」还原，保证起止时间不是同一时刻。
func (s *source) finishRun(tx *gormDB, task *asynctask.AsyncTask, r asynctask.Result, nowExpr string) (bool, error) {
	status := runStatusOf(r.Outcome)
	failure := string(r.Failure)
	if r.Outcome == asynctask.OutcomeSucceeded {
		failure = ""
	}
	errMsg := ""
	if r.Err != nil {
		errMsg = truncate(r.Err.Error(), 1024)
	}
	upd := fmt.Sprintf(`UPDATE %s
SET status = ?, failure_kind = ?, finished_at = %s, duration_ms = ?, result = ?, error_msg = ?
WHERE async_task_id = ? AND generation = ? AND attempt = ? AND status = 'running'`,
		CoreAsyncTaskRunTableName, nowExpr)
	res := tx.Exec(upd, status, failure, r.Duration.Milliseconds(), s.resultText(r.Payload), errMsg,
		task.ID, task.Generation, task.Attempt)
	if res.Error != nil {
		return false, res.Error
	}
	if res.RowsAffected > 0 {
		return true, nil
	}
	// 0 行：先分清「已被别人关闭」与「从来没有这一行」。
	exists, err := s.runExists(tx, task)
	if err != nil {
		return false, err
	}
	if exists {
		return false, nil
	}
	prefix, suffix := s.dialect.insertIgnoreClause()
	ins := fmt.Sprintf(`INSERT %sINTO %s
(id, created_at, async_task_id, generation, attempt, status, failure_kind, worker_id,
 started_at, finished_at, duration_ms, result, error_msg, request_id, trace_id)
VALUES (?, %s, ?, ?, ?, ?, ?, ?, %s, %s, ?, ?, ?, ?, ?)%s`,
		prefix, CoreAsyncTaskRunTableName, nowExpr,
		s.dialect.beforeExpr(), nowExpr, suffix)
	res = tx.Exec(ins,
		genUUID(), task.ID, task.Generation, task.Attempt, status, failure, task.WorkerID,
		s.dialect.beforeArg(r.Duration), r.Duration.Milliseconds(), s.resultText(r.Payload), errMsg,
		task.RequestID, task.TraceID,
	)
	if res.Error != nil {
		return false, res.Error
	}
	// 补插命中（1 行）说明这条 run 由本事务建立并关闭；冲突忽略（0 行）说明别人抢先建了。
	return res.RowsAffected > 0, nil
}

// runExists 判断本次 (task, generation, attempt) 的 run 行是否已存在。
func (s *source) runExists(tx *gormDB, task *asynctask.AsyncTask) (bool, error) {
	var id string
	sql := fmt.Sprintf(`SELECT id FROM %s WHERE async_task_id = ? AND generation = ? AND attempt = ?`,
		CoreAsyncTaskRunTableName)
	if err := tx.Raw(sql, task.ID, task.Generation, task.Attempt).Scan(&id).Error; err != nil {
		return false, err
	}
	return id != "", nil
}

// runStatusOf 把 Outcome 映射为尝试行状态。
func runStatusOf(o asynctask.Outcome) string {
	switch o {
	case asynctask.OutcomeSucceeded:
		return string(asynctask.StatusSucceeded)
	case asynctask.OutcomeSkipped:
		return string(asynctask.StatusSkipped)
	case asynctask.OutcomeCanceled:
		return string(asynctask.StatusCanceled)
	case asynctask.OutcomePaused:
		return string(asynctask.StatusPaused)
	default:
		return string(asynctask.StatusFailed)
	}
}

// resultText 把结果快照文本化；超过本 Source 的 MaxResultBytes（默认 4 KiB）按前缀截断。
//
// 截断不是错误：result 是业务快照，不是回调载荷。需要完整结果的调用方应走回调/外置文件，
// 调大 MaxResultBytes 只解决「快照够读」，不解决「半截 JSON 解析失败」。
func (s *source) resultText(b []byte) any {
	if len(b) == 0 {
		return nil
	}
	limit := DefaultMaxResultBytes
	if s != nil && s.maxResultBytes > 0 {
		limit = s.maxResultBytes
	}
	return truncate(string(b), limit)
}

// truncate 按字节上限截断字符串（保留前缀）。
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max]
}

// 时间来源约定（容易踩坑，写在这里）：
//   - 调度列（next_run_at / lease_until / started_at / finished_at）一律由 DB 表达式生成；
//   - Save 的事务内不混用应用时钟，避免多实例时钟偏差造成「租约刚写就已过期」。
