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

// reapStaleRuns 回收器 ①：收口已失去归属的尝试行（running 但任务侧已不归它）。
//
// 谓词关联 asyncTask 表：一次仍在正常续租、耗时超过 2×LeaseTTL 的尝试会被纯时间谓词
// 误判成残留——关联 asyncTask 表后，只要 asyncTask 仍 running 且 lease 未过期、attempt 一致，
// 尝试行就不算残留。finished_at 取失权时刻 LEAST(COALESCE(lease_until, now), now)。
//
// 返回处理的尝试行数。
func (s *store) reapStaleRuns(ctx context.Context, batch int) (int64, error) {
	if batch <= 0 {
		batch = 200
	}
	nowVal := s.dialect.nowValueExpr()

	// ① SELECT 候选（关联 asyncTask 表判归属）。
	selSQL := fmt.Sprintf(`SELECT r.id AS id, j.lease_until AS lease_until
FROM %s r JOIN %s j
  ON j.id = r.async_task_id AND j.generation = r.generation
WHERE r.status = 'running'
  AND (j.status <> 'running' OR %s OR j.attempt <> r.attempt)
LIMIT ?`,
		CoreAsyncTaskRunTableName, CoreAsyncTaskTableName, s.dialect.colLT("j.lease_until"))

	type staleRow struct {
		ID         string
		LeaseUntil *string
	}
	var stale []staleRow
	if err := s.db.WithContext(ctx).Raw(selSQL, batch).Scan(&stale).Error; err != nil {
		return 0, err
	}
	if len(stale) == 0 {
		return 0, nil
	}

	// 逐条带条件 UPDATE。
	var affected int64
	for _, r := range stale {
		// finished_at = LEAST(COALESCE(lease_until, now), now)
		finExpr, args := s.leastLeaseOrNow(r.LeaseUntil, nowVal)
		updSQL := fmt.Sprintf(`UPDATE %s
SET status = 'failed', failure_kind = 'lease_expired', finished_at = %s
WHERE id = ? AND status = 'running'`, CoreAsyncTaskRunTableName, finExpr)
		args = append(args, r.ID)
		res := s.db.WithContext(ctx).Exec(updSQL, args...)
		if res.Error != nil {
			return affected, res.Error
		}
		affected += res.RowsAffected
	}
	return affected, nil
}

// reapDeadAsyncTasks 回收器 ②：次数用尽且租约过期 → 判死。
//
// 清 worker_id/lease_until：否则僵尸进程的续租会成功。
// 返回处理的任务数。
func (s *store) reapDeadAsyncTasks(ctx context.Context, batch int) (int64, error) {
	if batch <= 0 {
		batch = 200
	}
	nowVal := s.dialect.nowValueExpr()

	selSQL := fmt.Sprintf(`SELECT id FROM %s
WHERE status = 'running' AND %s AND attempt >= max_attempts LIMIT ?`,
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
		updSQL := fmt.Sprintf(`UPDATE %s
SET status = 'failed', finished_at = %s, worker_id = '', lease_until = NULL
WHERE id = ? AND status = 'running' AND attempt >= max_attempts`, CoreAsyncTaskTableName, nowVal)
		res := s.db.WithContext(ctx).Exec(updSQL, id)
		if res.Error != nil {
			return affected, res.Error
		}
		affected += res.RowsAffected
	}
	return affected, nil
}

// reap 跑回收器两段（① 收口残留尝试行、② 判死次数用尽）。供后台循环与测试调用。
func (s *store) reap(ctx context.Context, batch int) (runs int64, asyncTasks int64, err error) {
	runs, err = s.reapStaleRuns(ctx, batch)
	if err != nil {
		return 0, 0, err
	}
	asyncTasks, err = s.reapDeadAsyncTasks(ctx, batch)
	if err != nil {
		return runs, 0, err
	}
	return runs, asyncTasks, nil
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
