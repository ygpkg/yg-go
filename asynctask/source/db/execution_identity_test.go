package db

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/gorm"
)

// 本文件覆盖「执行身份与预算」工作包的平台语义（设计 §9.3 Y3/Y5）：
//   - Y3 失败重试计数与执行序号分离：暂停不消耗额度、最后一次失败可终结；
//   - Y5 暂停结算（同事务核验租约 → 结算钩子 → 关闭 run → 回 pending）与失权结清。

// loadTaskRow 读回任务行（断言库内真实状态用）。
func loadTaskRow(t *testing.T, gdb *gorm.DB, id string) CoreAsyncTask {
	t.Helper()
	var row CoreAsyncTask
	if err := gdb.Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	return row
}

// loadRunRow 读回某次执行的尝试行。
func loadRunRow(t *testing.T, gdb *gorm.DB, id string, generation, attempt int) CoreAsyncTaskRun {
	t.Helper()
	var row CoreAsyncTaskRun
	if err := gdb.Where("async_task_id = ? AND generation = ? AND attempt = ?", id, generation, attempt).
		Take(&row).Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	return row
}

// TestPauseKeepsRetryQuota 暂停不消耗失败额度，且暂停后仍可被认领（设计 §9.2「暂停后仍可领取」）。
func TestPauseKeepsRetryQuota(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 2)

	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	// 显式给一个「已经到期」的恢复时刻：本用例只关心额度不被消耗，不关心暂停间隔
	// （零值 PauseUntil 会被兜成 now+DefaultMinPauseInterval，由
	// TestPauseZeroUntilStillWaitsMinInterval 覆盖）。
	due := time.Now().Add(-time.Second)
	if err := src.Save(ctx, task, asynctask.Result{
		Outcome:     asynctask.OutcomePaused,
		Failure:     asynctask.FailurePaused,
		Duration:    120 * time.Millisecond,
		PauseReason: "检查点已提交，让出算力",
		PauseUntil:  due,
	}, nil); err != nil {
		t.Fatalf("save paused: %v", err)
	}

	row := loadTaskRow(t, gdb, id)
	if row.Status != string(asynctask.StatusPending) {
		t.Fatalf("暂停后 status = %q, want pending", row.Status)
	}
	if row.FailureCount != 0 {
		t.Fatalf("暂停不该消耗失败额度，failure_count = %d", row.FailureCount)
	}
	if row.Attempt != 1 {
		t.Fatalf("暂停后 attempt 应保留为执行序号 1，got %d", row.Attempt)
	}
	if row.WorkerID != "" || row.LeaseUntil != nil {
		t.Fatalf("暂停后应清租约，got (%q,%v)", row.WorkerID, row.LeaseUntil)
	}
	if row.DurationMs != 120 {
		t.Fatalf("暂停应累计耗时，duration_ms = %d", row.DurationMs)
	}
	if row.LastError != "检查点已提交，让出算力" {
		t.Fatalf("暂停原因应落 last_error，got %q", row.LastError)
	}
	run := loadRunRow(t, gdb, id, 1, 1)
	if run.Status != string(asynctask.StatusPaused) || run.FailureKind != string(asynctask.FailurePaused) {
		t.Fatalf("暂停的尝试行 = (%q,%q), want (paused,paused)", run.Status, run.FailureKind)
	}

	// 暂停后仍可领取：额度 max=2 一次都没消耗。
	task2, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("暂停后应仍可认领: %v", err)
	}
	if task2.Attempt != 2 || task2.FailureCount != 0 {
		t.Fatalf("第二次认领后 (attempt,failure) = (%d,%d), want (2,0)", task2.Attempt, task2.FailureCount)
	}
	// 连续暂停也不消耗额度：再暂停一次，额度仍是 0。
	if err := src.Save(ctx, task2, asynctask.Result{Outcome: asynctask.OutcomePaused, PauseUntil: due}, nil); err != nil {
		t.Fatalf("save paused#2: %v", err)
	}
	if got := loadTaskRow(t, gdb, id); got.FailureCount != 0 {
		t.Fatalf("连续暂停仍不该消耗额度，failure_count = %d", got.FailureCount)
	}
}

// TestPauseSettlesCallbackInTx 暂停结算必须在同一事务内调用业务钩子（下游据此结算预留额），
// 钩子失败则整体回滚：任务仍是 running、run 仍 running、额度不变。
func TestPauseSettlesCallbackInTx(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)

	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	var sawTx atomic.Bool
	hookErr := errors.New("预算结算失败")
	err = src.Save(ctx, task, asynctask.Result{Outcome: asynctask.OutcomePaused}, func(cctx context.Context) error {
		if asynctask.TxFromContext(cctx) == nil {
			t.Error("暂停结算钩子必须拿到事务（TxFromContext）")
		}
		sawTx.Store(true)
		return hookErr
	})
	if !errors.Is(err, hookErr) {
		t.Fatalf("Save err = %v, want 钩子错误", err)
	}
	if !sawTx.Load() {
		t.Fatal("暂停必须调用结算钩子")
	}
	row := loadTaskRow(t, gdb, id)
	if row.Status != string(asynctask.StatusRunning) {
		t.Fatalf("钩子失败应回滚，status = %q, want running", row.Status)
	}
	if run := loadRunRow(t, gdb, id, 1, 1); run.Status != string(asynctask.StatusRunning) {
		t.Fatalf("钩子失败应回滚 run，status = %q, want running", run.Status)
	}
}

// TestPauseDelaySetsNextRunAt PauseUntil 会写进 next_run_at（让出后延迟恢复）。
func TestPauseDelaySetsNextRunAt(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)
	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	until := time.Now().Add(30 * time.Minute)
	if err := src.Save(ctx, task, asynctask.Result{
		Outcome:    asynctask.OutcomePaused,
		PauseUntil: until,
	}, nil); err != nil {
		t.Fatalf("save paused: %v", err)
	}
	row := loadTaskRow(t, gdb, id)
	if row.NextRunAt.Before(until.Add(-2 * time.Second)) {
		t.Fatalf("next_run_at = %v, 期望接近 %v", row.NextRunAt, until)
	}
	// 未到点不可认领。
	if _, err := src.Fetch(ctx, "t"); !errors.Is(err, asynctask.ErrNoAsyncTask) {
		t.Fatalf("未到恢复时间不该被认领，err = %v", err)
	}
}

// TestRetryCountsFailureAndKeepsAttempt 可重试失败：failure_count+1、attempt 保持（执行序号仍单调）。
func TestRetryCountsFailureAndKeepsAttempt(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)

	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := src.Save(ctx, task, asynctask.Result{
		Outcome: asynctask.OutcomeFailed,
		Failure: asynctask.FailureHandler,
		Err:     errors.New("boom"),
	}, nil); err != nil {
		t.Fatalf("save failed: %v", err)
	}
	row := loadTaskRow(t, gdb, id)
	if row.Status != string(asynctask.StatusPending) {
		t.Fatalf("可重试失败应回 pending，got %q", row.Status)
	}
	if row.FailureCount != 1 {
		t.Fatalf("failure_count = %d, want 1", row.FailureCount)
	}
	if row.Attempt != 1 {
		t.Fatalf("attempt 应保持 1（下一次认领再 +1），got %d", row.Attempt)
	}
}

// TestLastFailureTerminates 最后一次失败直接终结（设计 §9.2「最后一次失败可终结」）。
//
// 判据是 failure_count：第 max 次失败之后额度已尽，必须直接落终态。若这里判成「还能重试」，
// 任务会回 pending，而认领谓词（failure_count < max_attempts）又不成立——永久 pending，且判死
// 只扫 running，也不会兜住它。
// 注意：这个坑只在「预算与执行身份分离」之后才可能出现。旧口径里 attempt 同时兼任额度，
// 判定与认领同源，不会分裂——不要把这里写成「修了旧版的某个 bug」（release note 同理）。
func TestLastFailureTerminates(t *testing.T) {
	gdb := openTestDB(t)
	// 退避置零：本用例只关心额度判定，不想等真实退避。
	src, err := New(Config{
		DB:       gdb,
		LeaseTTL: 60 * time.Second,
		WorkerID: "w-test",
		Backoff:  func(int) time.Duration { return 0 },
	})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 2)

	for i := 0; i < 2; i++ {
		task, err := src.Fetch(ctx, "t")
		if err != nil {
			t.Fatalf("fetch#%d: %v", i+1, err)
		}
		if err := src.Save(ctx, task, asynctask.Result{
			Outcome: asynctask.OutcomeFailed,
			Failure: asynctask.FailureHandler,
			Err:     errors.New("boom"),
		}, nil); err != nil {
			t.Fatalf("save#%d: %v", i+1, err)
		}
	}
	row := loadTaskRow(t, gdb, id)
	if row.Status != string(asynctask.StatusFailed) {
		t.Fatalf("额度用尽的最后一次失败应终结，status = %q", row.Status)
	}
	if row.FailureCount != 2 || row.Attempt != 2 {
		t.Fatalf("(failure_count,attempt) = (%d,%d), want (2,2)", row.FailureCount, row.Attempt)
	}
	if _, err := src.Fetch(ctx, "t"); !errors.Is(err, asynctask.ErrNoAsyncTask) {
		t.Fatalf("终结后不该再被认领，err = %v", err)
	}
}

// TestTimeoutStillTerminal 超时（Y4 已具备）不改语义：最后一次超时落 StatusTimeout。
func TestTimeoutStillTerminal(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 1)
	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := src.Save(ctx, task, asynctask.Result{
		Outcome: asynctask.OutcomeFailed,
		Failure: asynctask.FailureTimeout,
		Err:     context.DeadlineExceeded,
	}, nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	if row := loadTaskRow(t, gdb, id); row.Status != string(asynctask.StatusTimeout) {
		t.Fatalf("status = %q, want timeout", row.Status)
	}
}

// TestReaperChargesFailureOnce reaper 与正常结算同时到达只扣一次（设计 §9.2 上游验证项 d）。
//
// 两条路径共用同一个守卫：`UPDATE run ... AND status='running'` 的命中行数决定谁扣费。
// 这里分别构造「reaper 先到」与「正常结算先到」，两种顺序下 failure_count 都必须恰好为 1。
func TestReaperChargesFailureOnce(t *testing.T) {
	t.Run("reaper_first", func(t *testing.T) {
		src, gdb := newLeaseTestSource(t)
		ctx := context.Background()
		id := seedPending(t, gdb, "t", 3)
		task, err := src.Fetch(ctx, "t")
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		// 租约过期：reaper 会把这次 run 当失权收口（并计一次失败）。
		if err := gdb.Exec(`UPDATE core_async_task SET lease_until = ? WHERE id = ?`,
			time.Now().Add(-time.Minute), id).Error; err != nil {
			t.Fatalf("expire lease: %v", err)
		}
		if runs, _, err := src.Reap(ctx, 10); err != nil || runs != 1 {
			t.Fatalf("reap runs=%d err=%v, want 1", runs, err)
		}
		if got := loadTaskRow(t, gdb, id); got.FailureCount != 1 {
			t.Fatalf("reaper 应计一次失败，failure_count = %d", got.FailureCount)
		}
		// 正常结算晚到：run 已被关闭 → 不再扣一次（仍可重试，额度只消耗 1）。
		if err := src.Save(ctx, task, asynctask.Result{
			Outcome: asynctask.OutcomeFailed,
			Failure: asynctask.FailureHandler,
			Err:     errors.New("boom"),
		}, nil); err != nil {
			t.Fatalf("save: %v", err)
		}
		if got := loadTaskRow(t, gdb, id); got.FailureCount != 1 {
			t.Fatalf("同时到达应只扣一次，failure_count = %d, want 1", got.FailureCount)
		}
	})

	t.Run("save_first", func(t *testing.T) {
		src, gdb := newLeaseTestSource(t)
		ctx := context.Background()
		id := seedPending(t, gdb, "t", 3)
		task, err := src.Fetch(ctx, "t")
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if err := src.Save(ctx, task, asynctask.Result{
			Outcome: asynctask.OutcomeFailed,
			Failure: asynctask.FailureHandler,
			Err:     errors.New("boom"),
		}, nil); err != nil {
			t.Fatalf("save: %v", err)
		}
		if runs, _, err := src.Reap(ctx, 10); err != nil {
			t.Fatalf("reap: %v", err)
		} else if runs != 0 {
			t.Fatalf("run 已由 Save 收口，reaper 不该再收口 %d 行", runs)
		}
		if got := loadTaskRow(t, gdb, id); got.FailureCount != 1 {
			t.Fatalf("同时到达应只扣一次，failure_count = %d, want 1", got.FailureCount)
		}
	})
}

// TestReaperSettlesReserveHook 失权回收要结清预留额：钩子在回收事务内被调用，
// 且钩子失败会让本次回收回滚（下一轮重试），不会出现「收口了 run 但没结清预算」。
func TestReaperSettlesReserveHook(t *testing.T) {
	gdb := openTestDB(t)
	ctx := context.Background()
	var calls atomic.Int32
	var fail atomic.Bool
	fail.Store(true)
	src, err := New(Config{
		DB:       gdb,
		LeaseTTL: 60 * time.Second,
		WorkerID: "w-test",
		ReclaimHook: func(hctx context.Context, run *asynctask.AsyncTaskRun, reason asynctask.FailureKind) error {
			calls.Add(1)
			if asynctask.TxFromContext(hctx) == nil {
				t.Error("回收钩子必须拿到事务")
			}
			if run.Generation != 1 || run.Attempt != 1 {
				t.Errorf("钩子拿到的执行身份 = (gen=%d, attempt=%d), want (1,1)", run.Generation, run.Attempt)
			}
			if reason != asynctask.FailureLeaseExpired {
				t.Errorf("回收原因 = %q, want lease_expired", reason)
			}
			if fail.Load() {
				return errors.New("结算失败")
			}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	id := seedPending(t, gdb, "t", 3)
	if _, err := src.Fetch(ctx, "t"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := gdb.Exec(`UPDATE core_async_task SET lease_until = ? WHERE id = ?`,
		time.Now().Add(-time.Minute), id).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	// 钩子报错 → 回收回滚：run 仍 running、failure_count 不变。
	if _, _, err := src.Reap(ctx, 10); err == nil {
		t.Fatal("钩子失败应让回收返回错误")
	}
	if run := loadRunRow(t, gdb, id, 1, 1); run.Status != string(asynctask.StatusRunning) {
		t.Fatalf("钩子失败应回滚 run，status = %q", run.Status)
	}
	if got := loadTaskRow(t, gdb, id); got.FailureCount != 0 {
		t.Fatalf("钩子失败应回滚额度，failure_count = %d", got.FailureCount)
	}

	// 钩子成功 → 收口 + 计费一次。
	fail.Store(false)
	if runs, _, err := src.Reap(ctx, 10); err != nil || runs != 1 {
		t.Fatalf("reap runs=%d err=%v, want 1", runs, err)
	}
	if got := loadTaskRow(t, gdb, id); got.FailureCount != 1 {
		t.Fatalf("failure_count = %d, want 1", got.FailureCount)
	}
	if calls.Load() < 2 {
		t.Fatalf("钩子调用次数 = %d, want >=2（失败重试一次）", calls.Load())
	}
}

// TestWithLeaseTx 持租约事务：持有租约时 fn 拿到事务；失权时返回 ErrLeaseLost 且不调用 fn。
func TestWithLeaseTx(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)

	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	called := false
	if err := src.WithLeaseTx(ctx, task, func(cctx context.Context) error {
		called = true
		tx, ok := asynctask.TxFromContext(cctx).(*gorm.DB)
		if !ok || tx == nil {
			t.Fatal("持租约事务必须给出 *gorm.DB")
		}
		// 事务内写自己的表（这里用 labels 代替下游的预算表）证明能参与同一事务。
		return tx.Exec(`UPDATE core_async_task SET labels = '{"k":"v"}' WHERE id = ?`, id).Error
	}); err != nil {
		t.Fatalf("with lease tx: %v", err)
	}
	if !called {
		t.Fatal("持租约时应调用 fn")
	}
	if row := loadTaskRow(t, gdb, id); row.Labels != `{"k":"v"}` {
		t.Fatalf("事务内写入未生效，labels = %q", row.Labels)
	}

	// 失权后：返回 ErrLeaseLost 且不调用 fn。
	if err := src.Release(ctx, task); err != nil {
		t.Fatalf("release: %v", err)
	}
	called = false
	err = src.WithLeaseTx(ctx, task, func(context.Context) error {
		called = true
		return nil
	})
	if !errors.Is(err, asynctask.ErrLeaseLost) {
		t.Fatalf("失权后 err = %v, want ErrLeaseLost", err)
	}
	if called {
		t.Fatal("失权时不得调用 fn")
	}
}

// TestTaskStatus 只读状态查询（取消补查用）。
func TestTaskStatus(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)

	status, err := src.TaskStatus(ctx, id)
	if err != nil || status != asynctask.StatusPending {
		t.Fatalf("TaskStatus = (%q,%v), want (pending,nil)", status, err)
	}
	if _, err := src.TaskStatus(ctx, "no-such-task"); !errors.Is(err, asynctask.ErrAsyncTaskNotFound) {
		t.Fatalf("不存在任务 err = %v, want ErrAsyncTaskNotFound", err)
	}
}

// TestCancelThenLateSaveDropped 取消后晚到的 Save 必须失权（清 worker_id 的守卫仍有效）。
func TestCancelThenLateSaveDropped(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)
	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := gdb.Exec(`UPDATE core_async_task SET status = 'canceled', worker_id = '', lease_until = NULL WHERE id = ?`, id).Error; err != nil {
		t.Fatalf("cancel: %v", err)
	}
	err = src.Save(ctx, task, asynctask.Result{Outcome: asynctask.OutcomeSucceeded}, nil)
	if !errors.Is(err, asynctask.ErrLeaseLost) {
		t.Fatalf("取消后 Save err = %v, want ErrLeaseLost", err)
	}
}

// zeroBackoffSource 造一个退避为 0 的 Source：额度判定类用例不想等真实退避。
func zeroBackoffSource(t *testing.T, gdb *gorm.DB) *Source {
	t.Helper()
	src, err := New(Config{
		DB:       gdb,
		LeaseTTL: 60 * time.Second,
		WorkerID: "w-test",
		Backoff:  func(int) time.Duration { return 0 },
	})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	return src
}

// TestRetryDecisionUsesLockedFailureCount 「重试 vs 终结」必须用行锁内读到的 failure_count，
// 不能用认领时的快照。
//
// 场景：回收器会在**另一个时刻**为更早那次崩溃的执行补记 failure_count（最多晚一个
// ReaperInterval=30s，常落在下一次执行期间）。task 上的值那时已经过时：若拿它判「还能重试」，
// 库里额度其实已经用尽 → 任务回 pending 后永远认领不到（认领谓词是 failure_count <
// max_attempts），也不会被判死（判死只看 running）→ 静默丢任务。
//
// 这里用「直接改库」模拟那笔补账（内存里的 task 仍是旧快照）。
func TestRetryDecisionUsesLockedFailureCount(t *testing.T) {
	t.Run("补账把额度吃满时直接终结", func(t *testing.T) {
		gdb := openTestDB(t)
		src := zeroBackoffSource(t, gdb)
		ctx := context.Background()
		id := seedPending(t, gdb, "t", 3)

		task, err := src.Fetch(ctx, "t")
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if task.FailureCount != 0 {
			t.Fatalf("认领时 failure_count = %d, want 0", task.FailureCount)
		}
		// 回收器补账：库里已是 2，内存快照仍是 0。
		if err := gdb.Exec(`UPDATE core_async_task SET failure_count = 2 WHERE id = ?`, id).Error; err != nil {
			t.Fatalf("simulate reclaim charge: %v", err)
		}
		if err := src.Save(ctx, task, asynctask.Result{
			Outcome: asynctask.OutcomeFailed,
			Failure: asynctask.FailureHandler,
			Err:     errors.New("boom"),
		}, nil); err != nil {
			t.Fatalf("save: %v", err)
		}

		row := loadTaskRow(t, gdb, id)
		if row.Status != string(asynctask.StatusFailed) {
			t.Fatalf("额度已用尽时应终结，status = %q（落 pending 即会永久卡住）", row.Status)
		}
		if row.FailureCount != 3 {
			t.Fatalf("failure_count = %d, want 3", row.FailureCount)
		}
	})

	t.Run("补账后仍有额度时重试且可再认领", func(t *testing.T) {
		gdb := openTestDB(t)
		src := zeroBackoffSource(t, gdb)
		ctx := context.Background()
		id := seedPending(t, gdb, "t", 3)

		task, err := src.Fetch(ctx, "t")
		if err != nil {
			t.Fatalf("fetch: %v", err)
		}
		if err := gdb.Exec(`UPDATE core_async_task SET failure_count = 1 WHERE id = ?`, id).Error; err != nil {
			t.Fatalf("simulate reclaim charge: %v", err)
		}
		if err := src.Save(ctx, task, asynctask.Result{
			Outcome: asynctask.OutcomeFailed,
			Failure: asynctask.FailureHandler,
			Err:     errors.New("boom"),
		}, nil); err != nil {
			t.Fatalf("save: %v", err)
		}
		row := loadTaskRow(t, gdb, id)
		if row.Status != string(asynctask.StatusPending) {
			t.Fatalf("status = %q, want pending", row.Status)
		}
		if row.FailureCount != 2 {
			t.Fatalf("failure_count = %d, want 2", row.FailureCount)
		}
		// 关键：必须真的还能被认领（额度 2 < 3），不能是「回 pending 但永远领不到」。
		task2, err := src.Fetch(ctx, "t")
		if err != nil {
			t.Fatalf("补账后仍应可认领: %v", err)
		}
		if task2.Attempt != 2 || task2.FailureCount != 2 {
			t.Fatalf("再次认领后 (attempt, failure) = (%d,%d), want (2,2)", task2.Attempt, task2.FailureCount)
		}
	})
}

// TestPauseZeroUntilStillWaitsMinInterval PauseUntil 零值（handler 只要求「让出算力」）也不能
// 立刻重新认领，否则暂停会按 PollInterval 忙等、run 表只增不减（见 DefaultMinPauseInterval）。
func TestPauseZeroUntilStillWaitsMinInterval(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	seedPending(t, gdb, "t", 3)

	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := src.Save(ctx, task, asynctask.Result{Outcome: asynctask.OutcomePaused}, nil); err != nil {
		t.Fatalf("save paused: %v", err)
	}
	// 立刻不可认领：证明 next_run_at 不是 now。
	if _, err := src.Fetch(ctx, "t"); !errors.Is(err, asynctask.ErrNoAsyncTask) {
		t.Fatalf("零值 PauseUntil 也不该被立刻重新认领，err = %v", err)
	}
	// 过了最短间隔后恢复可认领：证明不是把任务永久挂起。
	time.Sleep(DefaultMinPauseInterval + 200*time.Millisecond)
	if _, err := src.Fetch(ctx, "t"); err != nil {
		t.Fatalf("最短间隔之后应恢复可认领: %v", err)
	}
}
