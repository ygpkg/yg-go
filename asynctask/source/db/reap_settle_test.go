package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/gorm"
)

// 本文件覆盖回收器（reaper）的三处缺陷：
//   - 判死兜底 stranded pending（额度已空却停在 pending，既不执行也不会被 Cleanup 清理）；
//   - 回收器 ① 的逐行隔离（单行钩子失败不得挡住整段收口，更不得挡住 ② 判死）；
//   - 取消的执行按 canceled 结清（不能把「用户取消」当「进程崩溃」记账）。

// loadTaskRowForReap 读回任务行；后缀化的命名避免与并发编辑中的其它测试 helper 重名。
func loadTaskRowForReap(t *testing.T, gdb *gorm.DB, id string) CoreAsyncTask {
	t.Helper()
	var row CoreAsyncTask
	if err := gdb.Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatalf("load task: %v", err)
	}
	return row
}

// loadRunRowForReap 读回某次执行的尝试行。
func loadRunRowForReap(t *testing.T, gdb *gorm.DB, id string, generation, attempt int) CoreAsyncTaskRun {
	t.Helper()
	var row CoreAsyncTaskRun
	if err := gdb.Where("async_task_id = ? AND generation = ? AND attempt = ?", id, generation, attempt).
		Take(&row).Error; err != nil {
		t.Fatalf("load run: %v", err)
	}
	return row
}

// TestReapDeadCoversStrandedPending 额度已空却停在 pending 的任务必须被判死兜底。
//
// 这种行是「额度已空、状态是 pending」的残留：认领谓词 failure_count < max_attempts 永远
// 选不中它，Cleanup 又只删终态，不兜底就等于任务被静默丢掉。
// 谓词不能写成 `status IN ('running','pending') AND lease_until < now`——pending 行的
// lease_until 是 NULL，比较结果为 NULL，这类行永远选不出来。
func TestReapDeadCoversStrandedPending(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id := insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType: "invoice", Status: "pending", Attempt: 3,
		FailureCount: 3, MaxAttempts: 3,
		NextRunAt: time.Now().Add(-time.Minute),
	})

	if _, dead, err := s.reap(ctx, 10); err != nil {
		t.Fatalf("reap: %v", err)
	} else if dead != 1 {
		t.Fatalf("判死任务数 = %d, want 1（stranded pending 必须被兜底）", dead)
	}

	row := loadTaskRowForReap(t, s.db, id)
	if row.Status != string(asynctask.StatusFailed) {
		t.Fatalf("回收后 status = %q, want failed", row.Status)
	}
	if row.WorkerID != "" {
		t.Fatalf("判死后应清 worker_id，got %q", row.WorkerID)
	}
	if row.LeaseUntil != nil {
		t.Fatalf("判死后 lease_until 应为 NULL，got %v", row.LeaseUntil)
	}
}

// TestReapReclaimHookFailureDoesNotBlockDeadSweep ① 的钩子持续失败不得让 ② 一次都不跑。
//
// 否则一条永远无法结清的失权执行就能让「判死」永久失效：额度用尽的任务会一直卡在 running。
func TestReapReclaimHookFailureDoesNotBlockDeadSweep(t *testing.T) {
	gdb := openTestDB(t)
	ctx := context.Background()
	hookErr := errors.New("预算结算失败")
	src, err := New(Config{
		DB:       gdb,
		LeaseTTL: 60 * time.Second,
		WorkerID: "w-test",
		ReclaimHook: func(context.Context, *asynctask.AsyncTaskRun, asynctask.FailureKind) error {
			return hookErr
		},
	})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	id := seedPending(t, gdb, "t", 1)
	// 认领一次即崩溃：留着一条 running 尝试行，供 ① 命中（钩子失败 → 该行回滚）。
	if _, err := src.Fetch(ctx, "t"); err != nil {
		t.Fatalf("fetch: %v", err)
	}
	// 额度用尽 + 租约过期：① 会尝试收口这条 run，② 的判死条件同时成立。
	if err := gdb.Exec(`UPDATE core_async_task SET lease_until = ?, failure_count = 1 WHERE id = ?`,
		time.Now().Add(-time.Minute), id).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	runs, dead, err := src.Reap(ctx, 10)
	if err == nil {
		t.Fatal("钩子失败应回传错误")
	}
	if !errors.Is(err, hookErr) {
		t.Fatalf("err = %v, want 包含钩子错误", err)
	}
	if runs != 0 {
		t.Fatalf("① 收口行数 = %d, want 0（钩子失败应回滚这一行）", runs)
	}
	if dead != 1 {
		t.Fatalf("判死任务数 = %d, want 1（① 失败不得阻断 ②）", dead)
	}
	if row := loadTaskRowForReap(t, gdb, id); row.Status != string(asynctask.StatusFailed) {
		t.Fatalf("判死后 status = %q, want failed", row.Status)
	}
	// 逐行隔离的副作用：失败那一行确实回滚了（run 仍 running，等下一轮重试）。
	if run := loadRunRowForReap(t, gdb, id, 1, 1); run.Status != string(asynctask.StatusRunning) {
		t.Fatalf("钩子失败应回滚 run，status = %q", run.Status)
	}
}

// TestReapCanceledRunSettlesAsCanceled 被取消的执行要按 canceled 结清，不能按崩溃记账。
//
// 现场：Manager.Cancel 把任务置 canceled 并清租约，执行侧的 Save 因失权返回，
// 这次 run 行停在 running。回收器必须识别出「任务已取消」：run 落 canceled 分类、
// 钩子收到 FailureCanceled、且**不**消耗失败额度（取消不是失败）。
func TestReapCanceledRunSettlesAsCanceled(t *testing.T) {
	gdb := openTestDB(t)
	ctx := context.Background()
	var hookCalls int
	var hookRun *asynctask.AsyncTaskRun
	var hookReason asynctask.FailureKind
	src, err := New(Config{
		DB:       gdb,
		LeaseTTL: 60 * time.Second,
		WorkerID: "w-test",
		ReclaimHook: func(_ context.Context, run *asynctask.AsyncTaskRun, reason asynctask.FailureKind) error {
			hookCalls++
			hookRun = run
			hookReason = reason
			return nil
		},
	})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	id := seedPending(t, gdb, "t", 3)
	task, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	// 模拟 Manager.Cancel。
	if err := gdb.Exec(`UPDATE core_async_task SET status = 'canceled', worker_id = '', lease_until = NULL WHERE id = ?`,
		id).Error; err != nil {
		t.Fatalf("cancel: %v", err)
	}
	// 执行侧晚到的 Save 必须失权，不复活任务、也不关闭这次 run。
	if err := src.Save(ctx, task, asynctask.Result{Outcome: asynctask.OutcomeSucceeded}, nil); !errors.Is(err, asynctask.ErrLeaseLost) {
		t.Fatalf("取消后 Save err = %v, want ErrLeaseLost", err)
	}

	runs, _, err := src.Reap(ctx, 10)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if runs != 1 {
		t.Fatalf("收口行数 = %d, want 1（停 running 的取消执行）", runs)
	}
	if hookCalls != 1 {
		t.Fatalf("回收钩子调用次数 = %d, want 1", hookCalls)
	}
	if hookReason != asynctask.FailureCanceled {
		t.Fatalf("钩子收到的原因 = %q, want canceled", hookReason)
	}
	if hookRun == nil || hookRun.Failure != asynctask.FailureCanceled {
		t.Fatalf("钩子拿到的 run.Failure = %v, want canceled", hookRun)
	}

	run := loadRunRowForReap(t, gdb, id, 1, 1)
	// 取消的 run 行落 canceled（与 worker 自己结算 canceled 时 Save 写出的形态一致），
	// 不落 failed——否则按 status 统计「失败执行」的报表会把取消算进去。
	if run.Status != string(asynctask.StatusCanceled) {
		t.Fatalf("run status = %q, want canceled", run.Status)
	}
	if run.FailureKind != string(asynctask.FailureCanceled) {
		t.Fatalf("run failure_kind = %q, want canceled", run.FailureKind)
	}
	row := loadTaskRowForReap(t, gdb, id)
	if row.FailureCount != 0 {
		t.Fatalf("取消不消耗失败额度，failure_count = %d, want 0", row.FailureCount)
	}
	if row.Status != string(asynctask.StatusCanceled) {
		t.Fatalf("回收器不应改写终态，status = %q, want canceled", row.Status)
	}
}

// TestReapTakenOverRunStillChargesLeaseExpired 防回归：被接管后旧 run 仍按 lease_expired 收口。
//
// 任务行的 status 仍是 running、租约也被新执行者续上，只有 attempt 前进——这种「接管」现场
// 必须继续按崩溃记账（失败额度 +1），不能被「任务终态才不记账」的新逻辑影响。
func TestReapTakenOverRunStillChargesLeaseExpired(t *testing.T) {
	gdb := openTestDB(t)
	ctx := context.Background()
	var hookReasons []asynctask.FailureKind
	src, err := New(Config{
		DB:       gdb,
		LeaseTTL: 60 * time.Second,
		WorkerID: "w-A",
		ReclaimHook: func(_ context.Context, _ *asynctask.AsyncTaskRun, reason asynctask.FailureKind) error {
			hookReasons = append(hookReasons, reason)
			return nil
		},
	})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	id := seedPending(t, gdb, "t", 3)
	taskA, err := src.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch A: %v", err)
	}
	if taskA.Attempt != 1 {
		t.Fatalf("A 的 attempt = %d, want 1", taskA.Attempt)
	}
	// A 卡住（租约过期）→ B 接管：attempt 前进、租约重新有效。A 的 run 行仍停在 running。
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
	if taskB.Attempt != 2 {
		t.Fatalf("B 接管的 attempt = %d, want 2", taskB.Attempt)
	}

	runs, _, err := src.Reap(ctx, 10)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if runs != 1 {
		t.Fatalf("收口行数 = %d, want 1（A 的失权执行）", runs)
	}
	runA := loadRunRowForReap(t, gdb, id, 1, 1)
	if runA.Status != string(asynctask.StatusFailed) || runA.FailureKind != string(asynctask.FailureLeaseExpired) {
		t.Fatalf("A 的尝试行 = (%q,%q), want (failed,lease_expired)", runA.Status, runA.FailureKind)
	}
	if len(hookReasons) != 1 || hookReasons[0] != asynctask.FailureLeaseExpired {
		t.Fatalf("钩子收到的原因 = %v, want [lease_expired]", hookReasons)
	}
	// 接管是崩溃的一种：旧执行仍要吃一次失败额度。
	if row := loadTaskRowForReap(t, gdb, id); row.FailureCount != 1 {
		t.Fatalf("接管后旧执行应记一次失败，failure_count = %d, want 1", row.FailureCount)
	}
	if runB := loadRunRowForReap(t, gdb, id, 1, 2); runB.Status != string(asynctask.StatusRunning) {
		t.Fatalf("B 的尝试行应仍 running，got %q", runB.Status)
	}
}
