package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/gorm"
)

// RenewResult 是续租的结果信号，供调用方（Source.Renew）判断是否应取消 ctx。
type RenewResult int

const (
	// RenewOK 续租成功，仍持有租约。
	RenewOK RenewResult = iota
	// RenewCanceled 任务已被取消（canceled），调用方应立即取消 ctx。
	RenewCanceled
	// RenewLeaseExpired 已失权（被接管、已终态等），调用方应取消 ctx 并放弃写终态。
	RenewLeaseExpired
)

// terminalStatuses 是「可以回收/清理」的终态集合。新增终态时两处 SQL 都要跟着改。
var terminalStatuses = []string{
	string(asynctask.StatusSucceeded),
	string(asynctask.StatusFailed),
	string(asynctask.StatusCanceled),
	string(asynctask.StatusSkipped),
	string(asynctask.StatusTimeout),
}

// renew 续租一次：仅当仍是本 worker 持有且 running 时更新 lease_until=now+LeaseTTL。
//
// RowsAffected==0 说明已失权（被接管、被取消、已终态）。此时读回状态区分原因：
// canceled → RenewCanceled；其余 → RenewLeaseExpired。没有这条条件，
// Cancel 在跨实例上完全无效（internals「续租」）。
func (s *store) renew(ctx context.Context, id, workerID string) (RenewResult, error) {
	sql := fmt.Sprintf(`UPDATE %s SET lease_until = %s
WHERE id = ? AND worker_id = ? AND status = 'running'`,
		CoreAsyncTaskTableName, s.dialect.afterExpr())
	res := s.db.WithContext(ctx).Exec(sql, s.dialect.afterArg(s.leaseTTL), id, workerID)
	if res.Error != nil {
		return RenewOK, res.Error
	}
	if res.RowsAffected > 0 {
		return RenewOK, nil
	}
	// 失权：读回状态区分原因。
	var status string
	err := s.db.WithContext(ctx).Model(&CoreAsyncTask{}).Where("id = ?", id).
		Select("status").Scan(&status).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return RenewLeaseExpired, nil
		}
		return RenewOK, err
	}
	if status == string(asynctask.StatusCanceled) {
		return RenewCanceled, nil
	}
	return RenewLeaseExpired, nil
}

// staleRunRow 是回收器 ① 的候选行：被失权执行的那一行 run，加上任务侧租约（算失权时刻）。
type staleRunRow struct {
	ID          string
	AsyncTaskID string
	Generation  int32
	Attempt     int32
	WorkerID    string
	StartedAt   time.Time
	RequestID   string
	TraceID     string
	CreatedAt   time.Time
	LeaseUntil  *string
}

// reapStaleRuns 回收器 ①：收口已失去归属的尝试行（running 但任务侧已不归它）。
//
// 谓词关联 asyncTask 表：一次仍在正常续租、耗时超过 2×LeaseTTL 的尝试会被纯时间谓词
// 误判成残留——关联 asyncTask 表后，只要 asyncTask 仍 running 且 lease 未过期、attempt 一致，
// 尝试行就不算残留。finished_at 取失权时刻 LEAST(COALESCE(lease_until, now), now)。
//
// 失权回收同样要「结清」（设计 §9.2）：关闭一次失权执行时，同一个事务里
//   - 调用 reclaimHook 让下游按**预留额**结算阶段预算（进程已经不在，只有回收器能结这笔）；
//   - 若任务行仍在 running，failure_count + 1（崩溃/接管算一次失败，否则重启可无限刷额度）。
//
// 取消不是崩溃：任务被 Cancel 后 status 是 canceled，此时这次执行是「被取消」而不是
// 「进程没了」。两者的失败分类不同（FailureCanceled / FailureLeaseExpired）、是否记失败也不同，
// 所以这里把任务行的 status 一并锁出来做映射，否则下游会把用户取消当崩溃结算。
//
// 「只扣一次」：上面两件事都以「本次事务是否真的把 run 从 running 关闭」为前提
// （UPDATE ... AND status='running' 的命中行数），正常结算的 Save 用同一个守卫，
// 因此 reaper 与 Save 同时到达时只有一方会扣。
//
// 逐行隔离：单行事务失败只记下第一个错误并继续下一行。否则一条持续失败的
// ReclaimHook 行会让本段整体返回错误，连带把第二段「判死」也挡掉（一次都不跑）。
//
// 返回处理的尝试行数（只算真正由本段关闭的 run）。
func (s *store) reapStaleRuns(ctx context.Context, batch int) (int64, error) {
	if batch <= 0 {
		batch = 200
	}
	nowVal := s.dialect.nowValueExpr()

	// ① SELECT 候选（关联 asyncTask 表判归属）。
	selSQL := fmt.Sprintf(`SELECT r.id AS id, r.async_task_id AS async_task_id,
  r.generation AS generation, r.attempt AS attempt, r.worker_id AS worker_id,
  r.started_at AS started_at, r.request_id AS request_id, r.trace_id AS trace_id,
  r.created_at AS created_at, j.lease_until AS lease_until
FROM %s r JOIN %s j
  ON j.id = r.async_task_id AND j.generation = r.generation
WHERE r.status = 'running'
  AND (j.status <> 'running' OR %s OR j.attempt <> r.attempt)
LIMIT ?`,
		CoreAsyncTaskRunTableName, CoreAsyncTaskTableName, s.dialect.colLT("j.lease_until"))

	var stale []staleRunRow
	if err := s.db.WithContext(ctx).Raw(selSQL, batch).Scan(&stale).Error; err != nil {
		return 0, err
	}
	if len(stale) == 0 {
		return 0, nil
	}

	var affected int64
	var firstErr error
	for _, r := range stale {
		// closed 指出「本事务是否真的把这条 run 从 running 关闭」：只有提交成功且关闭了本次执行
		// 才计数，否则事务回滚后计数会虚高。
		var closed bool
		// 逐行一个事务：结清与收口必须原子，且下游钩子失败只回滚这一行（下一轮重试）。
		err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			// 先锁任务行再加锁尝试行：与 Save（lockOwnership 先锁任务行、再写 run 行）保持
			// 同样的加锁顺序，避免「reaper 先锁 run、Save 先锁 task」形成环形等待。
			// 连 status 一起取出：回收原因与「是否该记失败」都由任务行此刻的状态决定。
			lockSQL := fmt.Sprintf(`SELECT id, status FROM %s WHERE id = ?%s`,
				CoreAsyncTaskTableName, s.dialect.rowLockClause())
			var locked struct {
				ID     string
				Status string
			}
			if err := tx.Raw(lockSQL, r.AsyncTaskID).Scan(&locked).Error; err != nil {
				return err
			}
			// 取消不是崩溃：被 Cancel 的执行按 FailureCanceled 结清，下游据此区分
			// 「用户主动取消」与「执行进程消失」，不会把取消的预留额当成一次崩溃消耗。
			// run 行的 status 也跟着落 canceled（而不是一律 failed）：同一件事不该因为
			// 「这次 run 是 worker 自己结算的、还是回收器关的」而留下两种结局形态。
			runStatus := asynctask.StatusFailed
			reason := asynctask.FailureLeaseExpired
			if locked.Status == string(asynctask.StatusCanceled) {
				runStatus = asynctask.StatusCanceled
				reason = asynctask.FailureCanceled
			}
			// 只有任务行仍在 running 时才记一次失败：终态任务（含 canceled）这次执行是被
			// 正常结束/取消的，给它补失败额度会把「取消」错误地算进重试额度。
			chargeFailureQuota := locked.Status == string(asynctask.StatusRunning)

			// 参数顺序必须与 SQL 里的占位符一致：status 与 failure_kind 在最前，finished_at
			// 表达式自带的参数次之，最后是 WHERE 的 id。
			finExpr, finArgs := s.leastLeaseOrNow(r.LeaseUntil, nowVal)
			updSQL := fmt.Sprintf(`UPDATE %s
SET status = ?, failure_kind = ?, finished_at = %s
WHERE id = ? AND status = 'running'`, CoreAsyncTaskRunTableName, finExpr)
			args := make([]any, 0, len(finArgs)+3)
			args = append(args, string(runStatus), string(reason))
			args = append(args, finArgs...)
			args = append(args, r.ID)
			res := tx.Exec(updSQL, args...)
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				// 已被正常结算关闭：不重复计费、不重复结清。
				return nil
			}
			closed = true
			run := &asynctask.AsyncTaskRun{
				ID:          r.ID,
				AsyncTaskID: r.AsyncTaskID,
				Generation:  int(r.Generation),
				Attempt:     int(r.Attempt),
				Status:      runStatus,
				Failure:     reason,
				WorkerID:    r.WorkerID,
				StartedAt:   r.StartedAt,
				RequestID:   r.RequestID,
				TraceID:     r.TraceID,
				CreatedAt:   r.CreatedAt,
			}
			return s.settleReclaimedRun(ctx, tx, run, reason, chargeFailureQuota)
		})
		if err != nil {
			// 单行失败不中止整批：一次持续失败的钩子行只该拖住它自己这一行（下一轮重试），
			// 不该让其余残留执行和后面的「判死」都停下来。
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if closed {
			affected++
		}
	}
	return affected, firstErr
}

// settleReclaimedRun 在回收事务内结清一次失权执行：回调下游结清预留额，
// 并在还需要记失败额度时 failure_count + 1。
//
// chargeFailureQuota 由调用方按「任务行是否仍 running」决定：任务已进终态（典型是 canceled）
// 时只结清预留额、不记失败——否则「取消」会被下游当「崩溃」结算。
//
// failure_count 的更新带 generation 守卫：任务被 Replay 到新 generation 后，
// 旧 generation 的残留 run 不应再吃新轮次的额度。
func (s *store) settleReclaimedRun(ctx context.Context, tx *gorm.DB, run *asynctask.AsyncTaskRun, reason asynctask.FailureKind, chargeFailureQuota bool) error {
	if chargeFailureQuota {
		upd := fmt.Sprintf(`UPDATE %s SET failure_count = failure_count + 1
WHERE id = ? AND generation = ?`, CoreAsyncTaskTableName)
		if err := tx.Exec(upd, run.AsyncTaskID, run.Generation).Error; err != nil {
			return err
		}
	}
	if s.reclaimHook == nil {
		return nil
	}
	// 钩子拿到带事务的 ctx：下游在同一事务里按「预留额」扣减阶段预算。
	return s.reclaimHook(asynctask.WithTx(ctx, tx), run, reason)
}

// reapDeadAsyncTasks 回收器 ②：失败额度用尽 → 判死。
//
// 判据是 failure_count >= max_attempts（**不是** attempt）：attempt 是执行序号，
// 暂停会让它超过 max_attempts，但它不消耗额度，不能被判死。
// 清 worker_id/lease_until：否则僵尸进程的续租会成功。
//
// 判死要覆盖两种形态：
//   - running 且租约过期（执行进程消失）；
//   - pending 且额度已空（stranded）——这种任务认领谓词 failure_count < max_attempts 永远
//     选不中它，Cleanup 又只删终态，不兜底就等于静默丢任务。
//
// 两种形态不能合并成 `status IN ('running','pending') AND lease_until < now`：pending 行的
// lease_until 是 NULL，`NULL < now` 恒为 NULL，这类行永远选不出来。所以把时间条件只挂在
// running 分支上，靠 OR 的短路语义避开 NULL。
//
// 不写 last_error：判死不是新的失败，覆盖掉真实失败原因反而丢掉排查线索。
//
// 返回处理的任务数。
func (s *store) reapDeadAsyncTasks(ctx context.Context, batch int) (int64, error) {
	if batch <= 0 {
		batch = 200
	}
	nowVal := s.dialect.nowValueExpr()

	selSQL := fmt.Sprintf(`SELECT id FROM %s
WHERE failure_count >= max_attempts
  AND (status = 'pending' OR (status = 'running' AND %s))
LIMIT ?`,
		CoreAsyncTaskTableName, s.dialect.colLT("lease_until"))
	var ids []string
	if err := s.db.WithContext(ctx).Raw(selSQL, batch).Scan(&ids).Error; err != nil {
		return 0, err
	}
	if len(ids) == 0 {
		return 0, nil
	}
	var affected int64
	for _, id := range ids {
		// 守卫与 SELECT 同口径放宽到 pending：否则 SELECT 选出的 stranded pending 行
		// 会在 UPDATE 里命中 0 行，永远判不死。
		updSQL := fmt.Sprintf(`UPDATE %s
SET status = 'failed', finished_at = %s, worker_id = '', lease_until = NULL
WHERE id = ? AND failure_count >= max_attempts AND status IN ('running','pending')`,
			CoreAsyncTaskTableName, nowVal)
		res := s.db.WithContext(ctx).Exec(updSQL, id)
		if res.Error != nil {
			return affected, res.Error
		}
		affected += res.RowsAffected
	}
	return affected, nil
}

// reap 跑回收器两段（① 收口残留尝试行、② 判死次数用尽）。供后台循环与测试调用。
//
// 两段解耦：① 的错误（例如某行 ReclaimHook 持续失败）不得让 ② 一次都不跑——
// 否则「收口失败」会连带「判死失效」，额度用尽的任务永远卡在 running。
// 两段都跑完后用 errors.Join 把错误一并回传，由调用方（manager 维护循环）记录。
func (s *store) reap(ctx context.Context, batch int) (runs int64, asyncTasks int64, err error) {
	var errs []error
	runs, err = s.reapStaleRuns(ctx, batch)
	if err != nil {
		errs = append(errs, err)
	}
	asyncTasks, err = s.reapDeadAsyncTasks(ctx, batch)
	if err != nil {
		errs = append(errs, err)
	}
	return runs, asyncTasks, errors.Join(errs...)
}

// Cleanup 删除 before 之前进入终态的旧任务与其尝试记录（分批，最多若干批）。
//
// 删除条件钉在「终态 + finished_at」：pending/running 永不删；用 finished_at 而非
// created_at。清理在事务内先锁住候选任务行再删：否则「判到终态 → 被别人 Replay 回 pending
// 并重认领」之间，会把新一代正在跑的尝试行删掉（任务的 DELETE 有状态守卫活下来，
// run 行却已经被删了）。
func (s *store) Cleanup(ctx context.Context, before time.Time, batch int) (tasks int64, runs int64, err error) {
	if batch <= 0 {
		batch = 200
	}
	const maxBatches = 1000
	for i := 0; i < maxBatches; i++ {
		t, r, more, e := s.cleanupOnce(ctx, before, batch)
		if e != nil {
			return tasks, runs, e
		}
		tasks += t
		runs += r
		if !more {
			return tasks, runs, nil
		}
	}
	return tasks, runs, nil
}

// cleanupOnce 执行一批清理：SELECT 候选 → 事务内锁住仍符合条件的行 → 先删 run 再删 asyncTask。
// more 表示本批已满、可能还有。
func (s *store) cleanupOnce(ctx context.Context, before time.Time, batch int) (tasks, runs int64, more bool, err error) {
	statusList := terminalStatuses
	// ① SELECT 候选 id：终态且 finished_at < before。
	selSQL := fmt.Sprintf(`SELECT id FROM %s
WHERE status IN (%s) AND %s LIMIT ?`,
		CoreAsyncTaskTableName, wrapIDPlaceholders(len(statusList)), s.dialect.colLTBefore("finished_at"))
	selArgs := make([]any, 0, len(statusList)+1)
	for _, st := range statusList {
		selArgs = append(selArgs, st)
	}
	selArgs = append(selArgs, s.dialect.beforeArg(time.Since(before)), batch)
	var ids []string
	if err := s.db.WithContext(ctx).Raw(selSQL, selArgs...).Scan(&ids).Error; err != nil {
		return 0, 0, false, err
	}
	if len(ids) == 0 {
		return 0, 0, false, nil
	}
	// ② 事务内：重新锁定并确认仍是终态 → 删 run → 删 asyncTask。
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		lockSQL := fmt.Sprintf(`SELECT id FROM %s WHERE id IN (%s) AND status IN (%s)%s`,
			CoreAsyncTaskTableName, wrapIDPlaceholders(len(ids)), wrapIDPlaceholders(len(statusList)),
			s.dialect.rowLockClause())
		lockArgs := make([]any, 0, len(ids)+len(statusList))
		for _, id := range ids {
			lockArgs = append(lockArgs, id)
		}
		for _, st := range statusList {
			lockArgs = append(lockArgs, st)
		}
		var locked []string
		if err := tx.Raw(lockSQL, lockArgs...).Scan(&locked).Error; err != nil {
			return err
		}
		if len(locked) == 0 {
			return nil
		}
		r := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE async_task_id IN (%s)",
			CoreAsyncTaskRunTableName, wrapIDPlaceholders(len(locked))), toArgs(locked)...)
		if r.Error != nil {
			return r.Error
		}
		runs = r.RowsAffected
		j := tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE id IN (%s)",
			CoreAsyncTaskTableName, wrapIDPlaceholders(len(locked))), toArgs(locked)...)
		if j.Error != nil {
			return j.Error
		}
		tasks = j.RowsAffected
		return nil
	})
	if err != nil {
		return 0, 0, false, err
	}
	return tasks, runs, len(ids) == batch, nil
}

// toArgs 把字符串切片展开为 []any。
func toArgs(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}

var _ Store = (*store)(nil)
