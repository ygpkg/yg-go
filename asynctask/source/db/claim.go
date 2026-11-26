package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/gorm"
)

// store 是管理器内部的存储实现，承载认领、续租、释放、回收、清理与只读查询。
//
// 不在 manager 包外暴露具体类型：对外只经 Store 接口（只读 + Cleanup）与
// Manager 的写方法使用。两表无软删除——若用 gormdao 必须 WithoutSoftDelete()，
// 这里直接用 gorm 原生，避免误加 deleted_at IS NULL。
type store struct {
	db       *gorm.DB
	dialect  Dialect
	leaseTTL time.Duration
	// reclaimHook 是「失权回收」的事务内钩子（可空）：下游在这里结清阶段预算预留额。
	// 由 New 注入，不作为 newStore 的参数，避免改动既有测试构造点。
	reclaimHook asynctask.RunReclaimHook
}

// newStore 构造内部 store。
func newStore(db *gorm.DB, dialect Dialect, leaseTTL time.Duration) *store {
	return &store{db: db, dialect: dialect, leaseTTL: leaseTTL}
}

// claimBatch 用原生连接执行完整认领事务，确保：
//   - SET TRANSACTION ISOLATION LEVEL READ COMMITTED 在 START TRANSACTION 之前；
//   - 同一独占连接（s.db.Connection 绑定）。
//
// SQLite 无该语义：SET 语句为空、也无 SKIP LOCKED，退化为普通事务（测试不验并发）。
//
// 返回值是认领成功后的完整任务行（attempt 已 +1、status=running、worker_id 已写入），
// 认领与「建本次尝试的 running 行」在同一事务内完成，因此调用方无需再查一次。
func (s *store) claimBatch(ctx context.Context, taskType, workerID string, want int) ([]*CoreAsyncTask, error) {
	if want <= 0 {
		want = 50
	}
	var out []*CoreAsyncTask
	err := s.db.WithContext(ctx).Connection(func(tx *gorm.DB) error {
		// READ COMMITTED 必须在事务外设置（只对下一个事务生效）。
		if iso := s.dialect.isolationReadCommittedSQL(); iso != "" {
			if err := tx.Exec(iso).Error; err != nil {
				return err
			}
		}
		return tx.Transaction(func(itx *gorm.DB) error {
			rows, err := s.claimSelectAndFlip(itx, taskType, workerID, want)
			if err != nil {
				return err
			}
			if err := s.insertRunningRuns(itx, rows, workerID); err != nil {
				return err
			}
			out = rows
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// claimSelectAndFlip 在本连接的事务内执行两支 SELECT + 合并去重 + 带 workerID 的一次翻转。
//
// 两支认领分批各走各的索引（① idx_core_async_task_claim、② idx_core_async_task_status_lease_until），
// 合成一条 OR 会让两个索引无法同时生效，退化成按 task_type 区间逐行过滤（internals「认领」要点 1）。
//
// 认领的准入条件是 failure_count < max_attempts，**不是** attempt < max_attempts：
// attempt 只是单调执行序号（一次执行一行 run），主动暂停同样会让它 +1，
// 若拿它做准入，暂停就会被当成失败、把额度吃光（设计 §9.3 Y3）。
//
// ① 的索引是 (task_type, status, priority DESC, next_run_at)：ORDER BY 与索引列顺序/方向逐列一致，
// 因此不需要排序。代价是 next_run_at 的谓词退化为索引内过滤——若积压了大量「未来才到期」的高优先级
// 行，扫描会先跨过它们；这类积压目前不构成主要场景（延迟/退避行通常少数）。
//
// 合并时两支交替取（r1[0], r2[0], r1[1], r2[1]…）：只用 want=1 时若简单把 ① 排在前面，
// 队列一忙 ②（租约过期的接管候选）就永远轮不到，被接管的行会长时间卡在 running。
func (s *store) claimSelectAndFlip(tx *gorm.DB, taskType, workerID string, want int) ([]*CoreAsyncTask, error) {
	nowVal := s.dialect.nowValueExpr()
	lock := s.dialect.claimLockClause()

	// ① 待执行且到点：走 idx_core_async_task_claim（顺序即 priority DESC, next_run_at）
	// 任务链守卫：prev_step<0（首步/独立任务）直过；否则要求同 chain 的前序 step 已 succeeded。
	// 子查询走 idx_core_async_task_chain_id_step(chain_id, step)。
	sql1 := fmt.Sprintf(`SELECT * FROM %s j
WHERE j.task_type = ? AND j.status = 'pending' AND %s
  AND j.failure_count < j.max_attempts
  AND (j.prev_step < 0 OR EXISTS (
        SELECT 1 FROM %s p
        WHERE p.chain_id = j.chain_id AND p.step = j.prev_step AND p.status = 'succeeded'))
ORDER BY j.priority DESC, j.next_run_at
LIMIT ?%s`, CoreAsyncTaskTableName, s.dialect.colLE("next_run_at"), CoreAsyncTaskTableName, lock)

	// ② 执行中但租约过期（接管）：走 idx_core_async_task_status_lease_until
	sql2 := fmt.Sprintf(`SELECT * FROM %s
WHERE status = 'running' AND %s AND task_type = ?
  AND failure_count < max_attempts
ORDER BY lease_until
LIMIT ?%s`, CoreAsyncTaskTableName, s.dialect.colLT("lease_until"), lock)

	var r1, r2 []*CoreAsyncTask
	if err := tx.Raw(sql1, taskType, want).Scan(&r1).Error; err != nil {
		return nil, err
	}
	if err := tx.Raw(sql2, taskType, want).Scan(&r2).Error; err != nil {
		return nil, err
	}

	merged := interleaveByID(r1, r2, want)
	if len(merged) == 0 {
		return nil, nil
	}

	ids := make([]string, len(merged))
	args := make([]any, 0, len(merged)+3)
	args = append(args, workerID, s.dialect.afterArg(s.leaseTTL))
	for i, r := range merged {
		ids[i] = r.ID
		args = append(args, r.ID)
	}

	// 一次翻转：status='running', worker_id=?, attempt=attempt+1,
	// lease_until=now+LeaseTTL, started_at=COALESCE(started_at, now)。
	// failure_count 不动：它只在「可重试失败 / 失权回收」时由 Save 与 reaper 累加。
	// 带状态守卫：SELECT 与 UPDATE 之间若该行被 Cancel（SQLite 无行锁时可能发生），
	// 不能把它翻回 running「复活」执行。
	updSQL := fmt.Sprintf(`UPDATE %s
SET status = 'running', worker_id = ?, attempt = attempt + 1,
    lease_until = %s, started_at = COALESCE(started_at, %s)
WHERE id IN (%s) AND status IN ('pending','running')`,
		CoreAsyncTaskTableName, s.dialect.afterExpr(), nowVal, wrapIDPlaceholders(len(ids)))
	if err := tx.Exec(updSQL, args...).Error; err != nil {
		return nil, err
	}

	// 回填内存中的派生字段（DB 是权威，这里只是让调用方拿到「翻转后」的视图）。
	for _, r := range merged {
		r.Status = string(asynctask.StatusRunning)
		r.Attempt++
		r.WorkerID = workerID
	}
	return merged, nil
}

// interleaveByID 交替合并两支候选（r1/r2 各挑一条轮流取），去重后取前 want 条。
//
// 交替而非拼接，是为了让「租约过期可接管」的候选在有 pending 压力时也能被认领到。
func interleaveByID(r1, r2 []*CoreAsyncTask, want int) []*CoreAsyncTask {
	seen := make(map[string]struct{}, len(r1)+len(r2))
	out := make([]*CoreAsyncTask, 0, len(r1)+len(r2))
	pick := func(r *CoreAsyncTask) {
		if r == nil || len(out) >= want {
			return
		}
		if _, ok := seen[r.ID]; ok {
			return
		}
		seen[r.ID] = struct{}{}
		out = append(out, r)
	}
	for i := 0; len(out) < want && (i < len(r1) || i < len(r2)); i++ {
		if i < len(r1) {
			pick(r1[i])
		}
		if i < len(r2) {
			pick(r2[i])
		}
	}
	return out
}

// insertRunningRuns 为本次认领的每条任务写一行 running 尝试记录（started_at 取 DB 时钟）。
//
// 认领即建行，收尾时只 UPDATE，这样尝试记录的起止时间才是真实的；
// 进程崩溃时留下的 running 行也能被回收器 ① 收口。
// ID 由 Go 侧生成（跨方言没有可靠的 UUID 函数），因此按行插入：want 通常为 1。
func (s *store) insertRunningRuns(tx *gorm.DB, rows []*CoreAsyncTask, workerID string) error {
	if len(rows) == 0 {
		return nil
	}
	prefix, suffix := s.dialect.insertIgnoreClause()
	nowVal := s.dialect.nowValueExpr()
	sql := fmt.Sprintf(`INSERT %sINTO %s
(id, created_at, async_task_id, generation, attempt, status, failure_kind, worker_id,
 started_at, finished_at, duration_ms, result, error_msg, request_id, trace_id)
VALUES (?, %s, ?, ?, ?, 'running', '', ?, %s, NULL, 0, NULL, '', ?, ?)%s`,
		prefix, CoreAsyncTaskRunTableName, nowVal, nowVal, suffix)
	for _, r := range rows {
		if err := tx.Exec(sql,
			genUUID(), r.ID, r.Generation, r.Attempt, workerID,
			r.RequestID, r.TraceID,
		).Error; err != nil {
			return err
		}
	}
	return nil
}

// wrapIDPlaceholders 把 IN (...) 展开为 IN (?,?,...)，供原生 SQL 使用
// （gorm 原生 Raw 不展开切片）。
func wrapIDPlaceholders(n int) string {
	if n <= 0 {
		return "?"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

// loadAsyncTaskRow 按 id 读一行 core_async_task，不存在返回 gorm.ErrRecordNotFound。
func (s *store) loadAsyncTaskRow(ctx context.Context, id string) (*CoreAsyncTask, error) {
	var row CoreAsyncTask
	if err := s.db.WithContext(ctx).Where("id = ?", id).Take(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// 保持 asynctask 引用在编译期可见（转化函数在 convert.go）。
var _ = asynctask.ErrNoAsyncTask
