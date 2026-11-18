package manager

// 本期不带 worker 依赖（manager 不得 import asynctask/worker）。
//
// 源 source_test.go 里通过 worker.New + Run 的端到端用例
// （TestSource_EndToEnd_Succeeded、TestSource_OnSuccessTxFromContext）需要 import worker，
// 属「worker 集成」范畴，已迁至 worker 包侧测试，不在此移植。
// 这里保留不依赖 worker、只驱动 Manager 内部 dbSrc 的核心 Save 语义用例：
// HookTxError 回滚、LostLease 不覆盖/不跑钩子、ErrSkip→skipped。
//
// 另：源 TestSource_LostLease_DirectFinishAsyncTaskRowsAffected 直接调用 source/db 未导出
// 实现细节，属 DB 内部测试，未移植。

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"
	"gorm.io/gorm"
)

// newSourceTestManager 开内存库，构造 Manager 并返回底层 *gorm.DB。
func newSourceTestManager(t *testing.T) (*Manager, *gorm.DB) {
	t.Helper()
	gdb := openTestDB(t)
	cfg := defaultConfig()
	cfg.PollInterval = 500 * time.Millisecond
	m, err := New(cfg, gdb)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m, gdb
}

// submitAndClaim 提交一条任务并经本地 Source 认领（Fetch），返回 running 态的 *asynctask.AsyncTask。
func submitAndClaim(t *testing.T, m *Manager, taskType string) *asynctask.AsyncTask {
	t.Helper()
	ctx := context.Background()
	if _, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: taskType}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	var task *asynctask.AsyncTask
	deadline := time.Now().Add(2 * time.Second)
	for {
		var err error
		task, err = m.dbSrc.Fetch(ctx, taskType)
		if err == nil {
			break
		}
		if !errors.Is(err, asynctask.ErrNoAsyncTask) {
			t.Fatalf("fetch: %v", err)
		}
		if time.Now().After(deadline) {
			t.Fatalf("fetch: no task available within 2s")
		}
		time.Sleep(2 * time.Millisecond)
	}
	if task.Status != asynctask.StatusRunning || task.WorkerID == "" {
		t.Fatalf("after fetch: status=%q worker_id=%q, want running + worker_id", task.Status, task.WorkerID)
	}
	return task
}

// dbAsyncTaskStatus 直接读库里的 core_async_task 行，返回 (status, worker_id)。
func dbAsyncTaskStatus(t *testing.T, gdb *gorm.DB, id string) (string, string) {
	t.Helper()
	var row sourcedb.CoreAsyncTask
	if err := gdb.Where("id = ?", id).Take(&row).Error; err != nil {
		t.Fatalf("load asyncTask %s: %v", id, err)
	}
	return row.Status, row.WorkerID
}

// terminalRunCount 统计某任务下处于终态（非 running/pending）的尝试行数。
func terminalRunCount(t *testing.T, gdb *gorm.DB, asyncTaskID string) int64 {
	t.Helper()
	var n int64
	err := gdb.Model(&sourcedb.CoreAsyncTaskRun{}).
		Where("async_task_id = ? AND status IN ?", asyncTaskID,
			[]string{"succeeded", "failed", "canceled", "skipped"}).
		Count(&n).Error
	if err != nil {
		t.Fatalf("count terminal runs: %v", err)
	}
	return n
}

// TestSource_HookTxError_RollsBack 判据 1：OnSuccess 内（事务内回调）报错 → 整笔回滚。
func TestSource_HookTxError_RollsBack(t *testing.T) {
	m, gdb := newSourceTestManager(t)
	ctx := context.Background()
	task := submitAndClaim(t, m, "hooktx")

	hookErr := errors.New("on_success boom")
	cb := func(ctx context.Context) error { return hookErr }

	err := m.dbSrc.Save(ctx, task, asynctask.Result{
		Outcome:  asynctask.OutcomeSucceeded,
		Duration: time.Millisecond,
	}, cb)
	if !errors.Is(err, hookErr) {
		t.Fatalf("Save err = %v, want %v", err, hookErr)
	}

	if status, _ := dbAsyncTaskStatus(t, gdb, task.ID); status != string(asynctask.StatusRunning) {
		t.Fatalf("db status = %q, want running (rolled back)", status)
	}
	if n := terminalRunCount(t, gdb, task.ID); n != 0 {
		t.Fatalf("terminal run rows = %d, want 0 (rolled back)", n)
	}
	var total int64
	gdb.Model(&sourcedb.CoreAsyncTaskRun{}).Where("async_task_id = ?", task.ID).Count(&total)
	if total != 1 {
		t.Fatalf("run rows = %d, want 1 (claim-time running row kept, terminal update rolled back)", total)
	}
}

// TestSource_LostLease_NoOverwrite 判据 2：已失权后慢返回不覆盖终态、也不跑钩子。
func TestSource_LostLease_NoOverwrite(t *testing.T) {
	m, gdb := newSourceTestManager(t)
	ctx := context.Background()
	task := submitAndClaim(t, m, "lostlease")

	const otherWorker = "other-worker"
	if err := gdb.Exec(`UPDATE core_async_task SET worker_id = ? WHERE id = ?`, otherWorker, task.ID).Error; err != nil {
		t.Fatalf("simulate takeover: %v", err)
	}

	hookCalled := false
	err := m.dbSrc.Save(ctx, task, asynctask.Result{
		Outcome:  asynctask.OutcomeSucceeded,
		Duration: time.Millisecond,
	}, func(context.Context) error {
		hookCalled = true
		return nil
	})
	if !errors.Is(err, asynctask.ErrLeaseLost) {
		t.Fatalf("Save err = %v, want ErrLeaseLost", err)
	}
	if hookCalled {
		t.Fatal("失权后不得执行业务钩子（副作用无法回滚）")
	}

	status, worker := dbAsyncTaskStatus(t, gdb, task.ID)
	if status != string(asynctask.StatusRunning) {
		t.Fatalf("db status = %q, want running (terminal must not be overwritten)", status)
	}
	if worker != otherWorker {
		t.Fatalf("db worker_id = %q, want %q (must not be cleared)", worker, otherWorker)
	}
	if task.Status != asynctask.StatusRunning {
		t.Fatalf("in-memory task.Status = %q, want running (RowsAffected=0 branch)", task.Status)
	}
}

// TestSource_ErrSkip_Skipped 判据 3：Save(Result{Outcome:Skipped}) → 任务 status=skipped。
func TestSource_ErrSkip_Skipped(t *testing.T) {
	m, gdb := newSourceTestManager(t)
	ctx := context.Background()
	task := submitAndClaim(t, m, "skiptask")

	err := m.dbSrc.Save(ctx, task, asynctask.Result{
		Outcome:  asynctask.OutcomeSkipped,
		Failure:  asynctask.FailureSkipped,
		Duration: time.Millisecond,
	}, nil)
	if err != nil {
		t.Fatalf("Save err = %v", err)
	}
	if task.Status != asynctask.StatusSkipped {
		t.Fatalf("task.Status = %q, want skipped", task.Status)
	}
	status, worker := dbAsyncTaskStatus(t, gdb, task.ID)
	if status != string(asynctask.StatusSkipped) {
		t.Fatalf("db status = %q, want skipped", status)
	}
	if worker != "" {
		t.Fatalf("db worker_id = %q, want empty (cleared on finish)", worker)
	}
}
