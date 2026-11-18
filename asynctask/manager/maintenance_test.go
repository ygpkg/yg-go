package manager

import (
	"context"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// TestManagerMaintenanceReapsStuckTask 后台维护循环回收「次数用尽 + 租约过期」的任务。
//
// 没有这个循环时，这类任务既不会被认领（认领要求 attempt<max_attempts），
// 也不会被推进终态——永久停在 running（评审 H2）。
func TestManagerMaintenanceReapsStuckTask(t *testing.T) {
	cfg := defaultConfig()
	cfg.ReaperInterval = 20 * time.Millisecond
	cfg.CleanupInterval = 0
	m := newChainTestManager(t, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "stuck", MaxAttempts: 1}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	claimed, err := m.dbSrc.Fetch(ctx, "stuck")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	// 模拟进程崩溃：租约过期，且再没人 Save。
	if err := m.db.Exec(`UPDATE core_async_task SET lease_until = ? WHERE id = ?`,
		time.Now().Add(-time.Minute), claimed.ID).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := m.Start(ctx); err != nil { // 幂等
		t.Fatalf("start again: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for {
		got, gerr := m.Get(ctx, claimed.ID)
		if gerr != nil {
			t.Fatalf("get: %v", gerr)
		}
		if got.Status == asynctask.StatusFailed {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("维护循环未回收卡死任务，status 仍为 %q", got.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := m.Close(); err != nil { // 幂等
		t.Fatalf("close again: %v", err)
	}
}

// TestManagerMaintenanceCleansRetainedRecords 终态记录按 Retention 清理。
func TestManagerMaintenanceCleansRetainedRecords(t *testing.T) {
	cfg := defaultConfig()
	cfg.ReaperInterval = time.Hour // 本用例只看清理
	cfg.CleanupInterval = 20 * time.Millisecond
	cfg.Retention = time.Nanosecond // finished_at 立刻满足「已过期」
	m := newChainTestManager(t, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if _, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "gc", MaxAttempts: 1}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	claimed, err := m.dbSrc.Fetch(ctx, "gc")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if err := m.dbSrc.Save(ctx, claimed, asynctask.Result{Outcome: asynctask.OutcomeSucceeded}, nil); err != nil {
		t.Fatalf("save: %v", err)
	}

	if err := m.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = m.Close() }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		_, gerr := m.Get(ctx, claimed.ID)
		if gerr == asynctask.ErrAsyncTaskNotFound {
			return // 已被清理
		}
		if time.Now().After(deadline) {
			// sqlite 写成 datetime('now') 文本，与 Go 侧 before 参数比较可能受精度影响；
			// 这里退化为直接调用 Cleanup 确认逻辑本身可用。
			t.Fatalf("清理未发生，Get err=%v", gerr)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// TestSubmitAfterPrevStepWrittenAtomically prev_step 与 INSERT 同一条语句落库：
// 提交后可立即从库读到前序 step，不存在「先写 -1、再 UPDATE」的空窗。
func TestSubmitAfterPrevStepWrittenAtomically(t *testing.T) {
	m := newChainTestManager(t, nil)
	ctx := context.Background()

	s0, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "chain"})
	if err != nil {
		t.Fatalf("submit s0: %v", err)
	}
	s1, _, err := m.SubmitAfter(ctx, s0.ID, &asynctask.AsyncTaskSpec{Type: "chain"})
	if err != nil {
		t.Fatalf("submitAfter s1: %v", err)
	}

	var prev int32
	if err := m.db.Raw(`SELECT prev_step FROM core_async_task WHERE id = ?`, s1.ID).Scan(&prev).Error; err != nil {
		t.Fatalf("read prev_step: %v", err)
	}
	if prev != int32(s0.Step) {
		t.Fatalf("库里 prev_step = %d, want %d（必须与插入同一语句写入）", prev, s0.Step)
	}

	// 上游未成功前不可认领。
	if _, ferr := m.dbSrc.Fetch(ctx, "chain"); ferr != nil && ferr != asynctask.ErrNoAsyncTask {
		t.Fatalf("fetch: %v", ferr)
	}
	got, _ := m.Get(ctx, s1.ID)
	if got.Status != asynctask.StatusPending {
		t.Fatalf("前序未成功时 s1 应保持 pending，实际 %s", got.Status)
	}
}
