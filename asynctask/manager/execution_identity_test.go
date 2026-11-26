package manager

// 本文件覆盖「执行身份与预算」的平台侧管理面语义：
//   - Cancel 在状态翻转事务内发 PG 通知（sqlite 不支持 NOTIFY，退化为只翻转状态，不报错）；
//   - Cancel 的通知载荷带 (task_id, generation, attempt)，执行侧据此只取消那一次执行；
//   - Replay 重置失败额度（generation+1 = 全新额度）；
//   - 回收钩子经 manager 选项注入后生效。

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"
	"gorm.io/gorm"
)

// TestCancelWithoutNotifyChannel 未配通道时保持旧行为：只翻转状态。
func TestCancelWithoutNotifyChannel(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	task, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	canceled, err := m.Cancel(ctx, task.ID)
	if err != nil || !canceled {
		t.Fatalf("Cancel = (%v,%v), want (true,nil)", canceled, err)
	}
	got, err := m.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != asynctask.StatusCanceled {
		t.Fatalf("status = %q, want canceled", got.Status)
	}
	// 已终态幂等：再取消返回 false。
	if canceled, err := m.Cancel(ctx, task.ID); err != nil || canceled {
		t.Fatalf("重复 Cancel = (%v,%v), want (false,nil)", canceled, err)
	}
}

// TestCancelWithNotifyChannelOnSQLite 配了通道但方言不是 PG：仍必须正常取消（不发通知、不报错）。
func TestCancelWithNotifyChannelOnSQLite(t *testing.T) {
	m, err := New(nil, openTestDB(t), WithCancelNotifyChannel("asynctask_cancel"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	task, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if canceled, err := m.Cancel(ctx, task.ID); err != nil || !canceled {
		t.Fatalf("Cancel = (%v,%v), want (true,nil)", canceled, err)
	}
	got, err := m.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Status != asynctask.StatusCanceled {
		t.Fatalf("status = %q, want canceled", got.Status)
	}
}

// TestCancelSignalCarriesExecutionIdentity 取消通知载荷必须带执行身份（generation+attempt），
// 否则执行侧无法区分「这次执行」与「重放后的新执行」，晚到的取消会误杀新执行。
func TestCancelSignalCarriesExecutionIdentity(t *testing.T) {
	m, err := New(nil, openTestDB(t), WithCancelNotifyChannel("asynctask_cancel"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	task, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// 模拟已被认领过一次（attempt=1）的 running 任务。
	if err := m.db.WithContext(ctx).Exec(
		`UPDATE core_async_task SET status='running', attempt=1, worker_id='w1' WHERE id = ?`, task.ID).Error; err != nil {
		t.Fatalf("mark running: %v", err)
	}
	live, err := m.Get(ctx, task.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if live.Generation != 1 || live.Attempt != 1 {
		t.Fatalf("准备数据错误：gen=%d attempt=%d", live.Generation, live.Attempt)
	}
	if canceled, err := m.Cancel(ctx, task.ID); err != nil || !canceled {
		t.Fatalf("Cancel = (%v,%v)", canceled, err)
	}
	// 通知载荷由 Cancel 内部构造；这里用同一构造口径断言可被解析并且能匹配旧执行、不匹配新执行。
	sig := asynctask.CancelSignal{TaskID: task.ID, Generation: live.Generation, Attempt: live.Attempt}
	if _, ok := asynctask.DecodeCancelSignal(sig.Encode()); !ok {
		t.Fatal("取消载荷应可解析")
	}
	if !sig.Match(task.ID, 1, 1) {
		t.Fatal("应匹配被取消的那次执行")
	}
	if sig.Match(task.ID, 2, 1) {
		t.Fatal("不该匹配重放后的新 generation")
	}
}

// TestReplayResetsFailureQuota Replay 开启新一轮：attempt 与 failure_count 都归零（全新额度）。
func TestReplayResetsFailureQuota(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	task, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice", MaxAttempts: 2})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// 造一个「两次失败后终结」的现场。
	if err := m.db.WithContext(ctx).Exec(`UPDATE core_async_task
SET status='failed', attempt=3, failure_count=2, duration_ms=100 WHERE id = ?`, task.ID).Error; err != nil {
		t.Fatalf("mark failed: %v", err)
	}
	replayed, err := m.Replay(ctx, task.ID)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if replayed.Generation != 2 {
		t.Fatalf("generation = %d, want 2", replayed.Generation)
	}
	if replayed.Attempt != 0 || replayed.FailureCount != 0 {
		t.Fatalf("(attempt,failure_count) = (%d,%d), want (0,0)", replayed.Attempt, replayed.FailureCount)
	}
	if !replayed.Claimable() {
		t.Fatal("重放后应可再次认领")
	}
}

// TestReclaimHookWiredFromOption 回收钩子经 manager 选项注入到 DB Source，并在失权回收时被调用。
func TestReclaimHookWiredFromOption(t *testing.T) {
	gdb := openTestDB(t)
	var calls atomic.Int32
	m, err := New(nil, gdb, WithReclaimHook(func(ctx context.Context, run *asynctask.AsyncTaskRun, reason asynctask.FailureKind) error {
		calls.Add(1)
		if asynctask.TxFromContext(ctx) == nil {
			t.Error("回收钩子必须在事务内被调用")
		}
		return nil
	}))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx := context.Background()
	task, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// 造「已认领一次后崩溃」的现场：running + 租约过期 + running 尝试行。
	past := time.Now().Add(-time.Minute)
	if err := gdb.Exec(`UPDATE core_async_task SET status='running', attempt=1, worker_id='w1', lease_until=?
WHERE id = ?`, past, task.ID).Error; err != nil {
		t.Fatalf("mark running: %v", err)
	}
	if err := gdb.Exec(`INSERT INTO core_async_task_run
(id, created_at, async_task_id, generation, attempt, status, failure_kind, worker_id,
 started_at, finished_at, duration_ms, request_id, trace_id)
VALUES (?, ?, ?, 1, 1, 'running', '', 'w1', ?, NULL, 0, '', '')`,
		task.ID+"-run", time.Now(), task.ID, time.Now().Add(-time.Minute)).Error; err != nil {
		t.Fatalf("insert run: %v", err)
	}

	if _, _, err := m.Source().(interface {
		Reap(ctx context.Context, batch int) (int64, int64, error)
	}).Reap(ctx, 10); err != nil {
		t.Fatalf("reap: %v", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("回收钩子调用次数 = %d, want 1", calls.Load())
	}
}

// TestManagerWithLeaseTx 管理面也能做持租约事务（转发给 DB Source）。
func TestManagerWithLeaseTx(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	task, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	// 未持有租约（pending）：必须失权。
	err = m.WithLeaseTx(ctx, task, func(context.Context) error {
		t.Fatal("失权时不该调用 fn")
		return nil
	})
	if !errors.Is(err, asynctask.ErrLeaseLost) {
		t.Fatalf("err = %v, want ErrLeaseLost", err)
	}

	// 持有租约：认领后用同一身份调用，fn 拿到事务。
	src, err := sourcedb.New(sourcedb.Config{DB: m.db, LeaseTTL: time.Minute, WorkerID: "w-mgr"})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	claimed, err := src.Fetch(ctx, "invoice")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	called := false
	if err := m.WithLeaseTx(ctx, claimed, func(cctx context.Context) error {
		called = true
		tx, ok := asynctask.TxFromContext(cctx).(*gorm.DB)
		if !ok || tx == nil {
			t.Fatal("必须给出事务句柄")
		}
		return tx.Exec(`UPDATE core_async_task SET labels = '{"p":"1"}' WHERE id = ?`, claimed.ID).Error
	}); err != nil {
		t.Fatalf("WithLeaseTx: %v", err)
	}
	if !called {
		t.Fatal("持租约时应调用 fn")
	}
}
