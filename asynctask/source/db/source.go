package db

import (
	"context"
	"fmt"
	"strings"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/gorm"
)

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

// 编译期断言：source 满足 asynctask.Source。
var _ asynctask.Source = (*source)(nil)

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
//  2. 收尾本次尝试行；
//  3. 按结局推进任务状态，并在同一事务内调用业务钩子。
//
// 失权时不执行钩子、返回 asynctask.ErrLeaseLost：此时写终态会被守卫拦下，
// 钩子里的副作用却无法回滚，所以「不跑」比「跑了不记」正确。
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
		if err := s.finishRun(tx, task, r, now); err != nil {
			return err
		}

		switch r.Outcome {
		case asynctask.OutcomeSucceeded:
			if callback != nil {
				if err := callback(cctx); err != nil {
					return err
				}
			}
			return s.finishAsyncTask(tx, task, asynctask.StatusSucceeded, r, now)

		case asynctask.OutcomeCanceled:
			if callback != nil {
				_ = callback(cctx)
			}
			return s.finishAsyncTask(tx, task, asynctask.StatusCanceled, r, now)

		case asynctask.OutcomeSkipped:
			return s.finishAsyncTask(tx, task, asynctask.StatusSkipped, r, now)

		default: // Failed
			if callback != nil {
				if err := callback(cctx); err != nil {
					return err
				}
			}
			// 是否重试由次数决定（attempt 已在认领时 +1）。
			if task.HasRemainingAttempts() {
				return s.retryAsyncTask(tx, task, r)
			}
			// 终态状态：超时归为 StatusTimeout（独立状态），其余失败为 StatusFailed。
			termStatus := asynctask.StatusFailed
			if r.Failure == asynctask.FailureTimeout {
				termStatus = asynctask.StatusTimeout
			}
			if err := s.finishAsyncTask(tx, task, termStatus, r, now); err != nil {
				return err
			}
			// 失败级联：任务链某 step 失败后，同 chain 后续 step 置 canceled，
			// 避免其因前序永不 succeeded 而永久 pending 积压（仅 FailFastChain 开启时）。
			return s.cancelDownstream(tx, task, now)
		}
	})
}

// lockOwnership 锁住任务行并判断本次执行是否仍持有租约。
//
// 条件同时校验 worker_id / status / generation / attempt：只看 worker_id 时，
// 同一进程（共用 worker_id）的陈旧 goroutine 会把别人重认领后的任务当成自己的。
func (s *source) lockOwnership(tx *gormDB, task *asynctask.AsyncTask) (bool, error) {
	sql := fmt.Sprintf(`SELECT id FROM %s
WHERE id = ? AND worker_id = ? AND status = 'running' AND generation = ? AND attempt = ?%s`,
		CoreAsyncTaskTableName, s.dialect.rowLockClause())
	var id string
	if err := tx.Raw(sql, task.ID, task.WorkerID, task.Generation, task.Attempt).Scan(&id).Error; err != nil {
		return false, err
	}
	return id != "", nil
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
func (s *source) finishAsyncTask(tx *gormDB, task *asynctask.AsyncTask, status asynctask.Status, r asynctask.Result, nowExpr string) error {
	errMsg := ""
	if r.Err != nil {
		errMsg = truncate(r.Err.Error(), 1024)
	}
	sql := fmt.Sprintf(`UPDATE %s
SET status = ?, finished_at = %s, lease_until = NULL, worker_id = '',
    duration_ms = duration_ms + ?, last_error = ?, result = ?
WHERE id = ? AND worker_id = ? AND status = 'running' AND generation = ? AND attempt = ?`,
		CoreAsyncTaskTableName, nowExpr)
	res := tx.Exec(sql, string(status), r.Duration.Milliseconds(), errMsg, s.resultText(r.Payload),
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
func (s *source) retryAsyncTask(tx *gormDB, task *asynctask.AsyncTask, r asynctask.Result) error {
	backoff := s.backoff(task.Attempt)
	errMsg := ""
	if r.Err != nil {
		errMsg = truncate(r.Err.Error(), 1024)
	}
	sql := fmt.Sprintf(`UPDATE %s
SET status = 'pending', lease_until = NULL, worker_id = '',
    next_run_at = %s, duration_ms = duration_ms + ?, last_error = ?
WHERE id = ? AND worker_id = ? AND status = 'running' AND generation = ? AND attempt = ?`,
		CoreAsyncTaskTableName, s.dialect.afterExpr())
	res := tx.Exec(sql, s.dialect.afterArg(backoff), r.Duration.Milliseconds(), errMsg,
		task.ID, task.WorkerID, task.Generation, task.Attempt)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return asynctask.ErrLeaseLost
	}
	task.Status = asynctask.StatusPending
	return nil
}

// finishRun 收尾本次尝试行：认领时已插入 running 行，这里 UPDATE 成终态。
//
// 行不存在（历史数据、或 Source 不是本包写入的认领行）时补插一条，
// started_at 用 DB 表达式「now - 本次耗时」还原，保证起止时间不是同一时刻。
func (s *source) finishRun(tx *gormDB, task *asynctask.AsyncTask, r asynctask.Result, nowExpr string) error {
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
WHERE async_task_id = ? AND generation = ? AND attempt = ?`,
		CoreAsyncTaskRunTableName, nowExpr)
	res := tx.Exec(upd, status, failure, r.Duration.Milliseconds(), s.resultText(r.Payload), errMsg,
		task.ID, task.Generation, task.Attempt)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected > 0 {
		return nil
	}
	prefix, suffix := s.dialect.insertIgnoreClause()
	ins := fmt.Sprintf(`INSERT %sINTO %s
(id, created_at, async_task_id, generation, attempt, status, failure_kind, worker_id,
 started_at, finished_at, duration_ms, result, error_msg, request_id, trace_id)
VALUES (?, %s, ?, ?, ?, ?, ?, ?, %s, %s, ?, ?, ?, ?, ?)%s`,
		prefix, CoreAsyncTaskRunTableName, nowExpr,
		s.dialect.beforeExpr(), nowExpr, suffix)
	return tx.Exec(ins,
		genUUID(), task.ID, task.Generation, task.Attempt, status, failure, task.WorkerID,
		s.dialect.beforeArg(r.Duration), r.Duration.Milliseconds(), s.resultText(r.Payload), errMsg,
		task.RequestID, task.TraceID,
	).Error
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
