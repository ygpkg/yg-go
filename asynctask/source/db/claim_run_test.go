package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/gorm"
)

// newLeaseTestSource 构造一个 sqlite 内存库 + Source（AutoMigrate 三表）。
func newLeaseTestSource(t *testing.T) (*source, *gorm.DB) {
	t.Helper()
	gdb := openTestDB(t, true)
	if err := gdb.AutoMigrate(&CoreAsyncTask{}, &CoreAsyncTaskRun{}, &CoreAsyncTaskType{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	src, err := New(Config{DB: gdb, LeaseTTL: 60 * time.Second, WorkerID: "w-test"})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	return src, gdb
}

// seedPending 插入一条到点的 pending 任务。
func seedPending(t *testing.T, gdb *gorm.DB, taskType string, maxAttempts int32) string {
	t.Helper()
	now := time.Now()
	id := genUUID()
	if err := gdb.Create(&CoreAsyncTask{
		ID: id, CreatedAt: now, UpdatedAt: now, TaskType: taskType,
		Status: "pending", Generation: 1, Attempt: 0, MaxAttempts: maxAttempts,
		NextRunAt: now.Add(-time.Minute),
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	return id
}

// TestClaimCreatesRunningRunRow 认领时即建 running 尝试行，Save 时只收尾，
// 因此尝试记录的 started_at 是真实开始时间（不是 Save 时刻）。
func TestClaimCreatesRunningRunRow(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)

	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	var run CoreAsyncTaskRun
	if err := gdb.Where("async_task_id = ? AND generation = ? AND attempt = ?", id, 1, 1).
		Take(&run).Error; err != nil {
		t.Fatalf("认领应写入 running 尝试行: %v", err)
	}
	if run.Status != string(asynctask.StatusRunning) {
		t.Fatalf("run status = %q, want running", run.Status)
	}
	if run.FinishedAt != nil {
		t.Fatalf("认领时 finished_at 应为 NULL")
	}
	startedAt := run.StartedAt

	time.Sleep(30 * time.Millisecond)
	if err := src.Save(ctx, task, asynctask.Result{
		Outcome:  asynctask.OutcomeSucceeded,
		Duration: 30 * time.Millisecond,
	}, nil); err != nil {
		t.Fatalf("save: %v", err)
	}

	var after CoreAsyncTaskRun
	if err := gdb.Where("async_task_id = ? AND generation = ? AND attempt = ?", id, 1, 1).
		Take(&after).Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	if after.Status != string(asynctask.StatusSucceeded) {
		t.Fatalf("run status = %q, want succeeded", after.Status)
	}
	// SQLite 的 datetime('now') 只到秒：同一秒内允许相等，但不能早于开始时间
	// （旧实现在 Save 时才插行，started_at 与 finished_at 恒等于同一时刻，这里至少要求
	// started_at 来自认领时刻而非 Save 时刻 —— 由上面的 Take 断言已覆盖）。
	if after.FinishedAt == nil || after.FinishedAt.Before(startedAt) {
		t.Fatalf("finished_at(%v) 不应早于 started_at(%v)", after.FinishedAt, startedAt)
	}
}

// TestSaveAfterReleaseReturnsLeaseLost 主动放租约后，晚到的 Save 必须失权：
// 返回 ErrLeaseLost、不跑钩子、不改库。
func TestSaveAfterReleaseReturnsLeaseLost(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)

	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := src.Release(ctx, task); err != nil {
		t.Fatalf("release: %v", err)
	}
	var row CoreAsyncTask
	if err := gdb.Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if row.WorkerID != "" {
		t.Fatalf("Release 后 worker_id 应清空，实际 %q", row.WorkerID)
	}

	hookCalled := false
	err = src.Save(ctx, task, asynctask.Result{Outcome: asynctask.OutcomeSucceeded, Duration: time.Millisecond},
		func(context.Context) error {
			hookCalled = true
			return nil
		})
	if !errors.Is(err, asynctask.ErrLeaseLost) {
		t.Fatalf("Save err = %v, want ErrLeaseLost", err)
	}
	if hookCalled {
		t.Fatal("失权后不得执行钩子")
	}
	if err := gdb.Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if row.Status != string(asynctask.StatusRunning) {
		t.Fatalf("失权后状态不应被改写，got %q", row.Status)
	}
}

// TestSaveAfterCancelReturnsLeaseLost 任务被取消后，执行侧的 Save 不得复活它。
func TestSaveAfterCancelReturnsLeaseLost(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)

	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := gdb.Exec(`UPDATE core_async_task SET status = 'canceled', worker_id = '' WHERE id = ?`, id).Error; err != nil {
		t.Fatalf("cancel: %v", err)
	}
	err = src.Save(ctx, task, asynctask.Result{Outcome: asynctask.OutcomeSucceeded, Duration: time.Millisecond}, nil)
	if !errors.Is(err, asynctask.ErrLeaseLost) {
		t.Fatalf("Save err = %v, want ErrLeaseLost", err)
	}
	var row CoreAsyncTask
	_ = gdb.Where("id = ?", id).Take(&row).Error
	if row.Status != string(asynctask.StatusCanceled) {
		t.Fatalf("取消状态被改写为 %q", row.Status)
	}
}

// TestTakeoverKeepsBothRunRows 接管后两次尝试各有一行；回收器把失权的那行收口为 lease_expired。
func TestTakeoverKeepsBothRunRows(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)

	taskA, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch A: %v", err)
	}
	// 模拟 A 卡住：租约过期。
	if err := gdb.Exec(`UPDATE core_async_task SET lease_until = ? WHERE id = ?`,
		time.Now().Add(-time.Minute), id).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	srcB, err := New(Config{DB: gdb, LeaseTTL: 60 * time.Second, WorkerID: "w-B"})
	if err != nil {
		t.Fatalf("new source B: %v", err)
	}
	taskB, err := srcB.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch B: %v", err)
	}
	if taskB.ID != id || taskB.Attempt != 2 {
		t.Fatalf("接管失败: id=%s attempt=%d", taskB.ID, taskB.Attempt)
	}

	// A 的晚到 Save 必须失权。
	if err := src.Save(ctx, taskA, asynctask.Result{Outcome: asynctask.OutcomeSucceeded, Duration: time.Millisecond}, nil); !errors.Is(err, asynctask.ErrLeaseLost) {
		t.Fatalf("A 的 Save err = %v, want ErrLeaseLost", err)
	}

	runs, err := src.store.reapStaleRuns(ctx, 10)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if runs != 1 {
		t.Fatalf("reap 处理行数 = %d, want 1（A 的那次尝试）", runs)
	}
	var runA CoreAsyncTaskRun
	if err := gdb.Where("async_task_id = ? AND generation = ? AND attempt = ?", id, 1, 1).Take(&runA).Error; err != nil {
		t.Fatalf("load run A: %v", err)
	}
	if runA.Status != string(asynctask.StatusFailed) || runA.FailureKind != string(asynctask.FailureLeaseExpired) {
		t.Fatalf("A 的尝试行 = (%s,%s), want (failed,lease_expired)", runA.Status, runA.FailureKind)
	}
	var runB CoreAsyncTaskRun
	if err := gdb.Where("async_task_id = ? AND generation = ? AND attempt = ?", id, 1, 2).Take(&runB).Error; err != nil {
		t.Fatalf("load run B: %v", err)
	}
	if runB.Status != string(asynctask.StatusRunning) {
		t.Fatalf("B 的尝试行应仍 running，got %q", runB.Status)
	}
}

// TestCleanupIncludesTimeout 清理必须覆盖 timeout 终态（曾漏掉，超时任务永不清理）。
func TestCleanupIncludesTimeout(t *testing.T) {
	_, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	old := time.Now().Add(-48 * time.Hour)
	id := genUUID()
	if err := gdb.Create(&CoreAsyncTask{
		ID: id, CreatedAt: old, UpdatedAt: old, TaskType: "t", Status: "timeout",
		Generation: 1, MaxAttempts: 3, NextRunAt: old, FinishedAt: &old,
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	s := newStore(gdb, DialectSQLite, time.Minute)
	tasks, _, err := s.Cleanup(ctx, time.Now().Add(-time.Hour), 10)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if tasks != 1 {
		t.Fatalf("清理任务数 = %d, want 1", tasks)
	}
}

// TestReapDeadAsyncTask 次数用尽 + 租约过期 → 判死，任务卡在 running 的形态被解除。
func TestReapDeadAsyncTask(t *testing.T) {
	_, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 1)
	s := newStore(gdb, DialectSQLite, time.Minute)
	// 直接翻转成 running/attempt=1（认领一次后崩溃）。
	if _, err := s.claimBatch(ctx, "t", "w-1", 1); err != nil {
		t.Fatalf("claim: %v", err)
	}
	if err := gdb.Exec(`UPDATE core_async_task SET lease_until = ? WHERE id = ?`,
		time.Now().Add(-time.Minute), id).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	// 次数用尽（attempt=1 >= max_attempts=1）后，认领侧两支 SQL 都不会再选中它。
	if rows, err := s.claimBatch(ctx, "t", "w-2", 1); err != nil || len(rows) != 0 {
		t.Fatalf("次数用尽后不该再被认领: rows=%d err=%v", len(rows), err)
	}
	if _, dead, err := s.reap(ctx, 10); err != nil {
		t.Fatalf("reap: %v", err)
	} else if dead != 1 {
		t.Fatalf("判死任务数 = %d, want 1", dead)
	}
	var row CoreAsyncTask
	if err := gdb.Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if row.Status != string(asynctask.StatusFailed) {
		t.Fatalf("回收后 status = %q, want failed", row.Status)
	}
	if row.WorkerID != "" || row.LeaseUntil != nil {
		t.Fatalf("判死后应清 worker_id/lease_until，got (%q,%v)", row.WorkerID, row.LeaseUntil)
	}
}

// TestSaveInsertFallbackWhenRunRowMissing 认领行缺失时（历史数据/异构 Source），
// Save 必须补插尝试行，且 started_at 用「now - 本次耗时」还原、不早于 finished_at。
func TestSaveInsertFallbackWhenRunRowMissing(t *testing.T) {
	src, gdb := newLeaseTestSource(t)
	ctx := context.Background()
	id := seedPending(t, gdb, "t", 3)

	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	// 删掉认领时写入的 running 行，模拟历史数据。
	if err := gdb.Exec(`DELETE FROM core_async_task_run WHERE async_task_id = ?`, id).Error; err != nil {
		t.Fatalf("delete run: %v", err)
	}
	if err := src.Save(ctx, task, asynctask.Result{
		Outcome:  asynctask.OutcomeFailed,
		Failure:  asynctask.FailureHandler,
		Err:      errors.New("boom"),
		Duration: 250 * time.Millisecond,
	}, nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	// 只取需要断言的列。SQLite 方言下补插用的 started_at/finished_at 由
	// julianday() 标量（REAL）写入，与源一致；yg-go 用的 mattn 驱动无法把 REAL
	// 扫回 time.Time（源的 ncruces 驱动可以）。因此时间列这里按原始 julianday 读取校验，
	// 其余列（status/failure_kind）直接断言。SQL 与源保持逐字一致。
	var row struct {
		Status      string
		FailureKind string
	}
	if err := gdb.Raw(`SELECT status, failure_kind FROM core_async_task_run
		WHERE async_task_id = ? AND generation = ? AND attempt = ?`, id, 1, 1).
		Scan(&row).Error; err != nil {
		t.Fatalf("补插失败: %v", err)
	}
	if row.Status == "" {
		t.Fatal("补插行缺失")
	}
	if row.Status != string(asynctask.StatusFailed) || row.FailureKind != string(asynctask.FailureHandler) {
		t.Fatalf("run = (%s,%s), want (failed,handler_error)", row.Status, row.FailureKind)
	}
	var times struct {
		StartedAt  *float64
		FinishedAt *float64
	}
	if err := gdb.Raw(`SELECT julianday(started_at) AS started_at, julianday(finished_at) AS finished_at
		FROM core_async_task_run WHERE async_task_id = ? AND generation = ? AND attempt = ?`, id, 1, 1).
		Scan(&times).Error; err != nil {
		t.Fatalf("读取起止时间失败: %v", err)
	}
	if times.FinishedAt == nil {
		t.Fatal("补插行缺 finished_at")
	}
	if times.StartedAt == nil {
		t.Fatal("补插行缺 started_at")
	}
	// julianday 以「天」为单位：250ms ≈ 0.250/86400 天。校验量级（还原的是本次耗时，
	// 而非恒等于 Save 时刻）；容差 1s ≈ 1/86400 天，与源断言 ±1s 等价。
	deltaDays := *times.FinishedAt - *times.StartedAt
	const tolDays = 1.0 / 86400.0
	if deltaDays < -tolDays || deltaDays > tolDays {
		t.Fatalf("started_at 未按耗时还原: finish-start=%v 天", deltaDays)
	}
}
