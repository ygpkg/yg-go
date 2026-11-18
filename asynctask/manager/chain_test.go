package manager

import (
	"context"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// forceStatus 直接把某 asyncTask 置为指定终态（模拟前置步骤执行成功/失败）。
func forceStatus(t *testing.T, m *Manager, asyncTaskID string, st asynctask.Status) {
	t.Helper()
	now := time.Now()
	err := m.db.Exec("UPDATE core_async_task SET status = ?, finished_at = ? WHERE id = ?",
		string(st), now, asyncTaskID).Error
	if err != nil {
		t.Fatalf("forceStatus: %v", err)
	}
}

// TestChain_SubmitAfter 验证三步链的字段继承与「前序未成功不可认领」守卫。
func TestChain_SubmitAfter(t *testing.T) {
	m := newChainTestManager(t, nil)
	ctx := context.Background()

	// 首步
	s0, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "chain", Payload: []byte("s0")})
	if err != nil {
		t.Fatalf("submit s0: %v", err)
	}
	if s0.ChainID != "" || s0.Step != 0 || s0.PrevStep != -1 {
		t.Fatalf("首步字段错: chain=%q step=%d prev=%d", s0.ChainID, s0.Step, s0.PrevStep)
	}

	// 第二步：继承链
	s1, _, err := m.SubmitAfter(ctx, s0.ID, &asynctask.AsyncTaskSpec{Type: "chain", Payload: []byte("s1")})
	if err != nil {
		t.Fatalf("submitAfter s1: %v", err)
	}
	if s1.ChainID != s0.ID || s1.Step != 1 || s1.PrevStep != 0 {
		t.Fatalf("s1 字段错: chain=%q(期望%s) step=%d prev=%d", s1.ChainID, s0.ID, s1.Step, s1.PrevStep)
	}
	// 第三步
	s2, _, err := m.SubmitAfter(ctx, s1.ID, &asynctask.AsyncTaskSpec{Type: "chain", Payload: []byte("s2")})
	if err != nil {
		t.Fatalf("submitAfter s2: %v", err)
	}
	if s2.Step != 2 || s2.PrevStep != 1 {
		t.Fatalf("s2 字段错: step=%d prev=%d", s2.Step, s2.PrevStep)
	}

	// 认领守卫：s0 可认领，s1/s2 因前序未 succeeded 不可认领。
	claimed, err := m.dbSrc.Fetch(ctx, "chain")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if claimed.ID != s0.ID {
		t.Fatalf("首次认领应为 s0(%s)，实际 %s", s0.ID, claimed.ID)
	}
	// s0 认领后还在 running；s1 仍不可认领（前序未 succeeded）。
	if _, err := m.dbSrc.Fetch(ctx, "chain"); err != asynctask.ErrNoAsyncTask {
		t.Fatalf("s0 未成功时 s1 不应可认领，err=%v", err)
	}

	// s0 置 succeeded → s1 可认领。
	forceStatus(t, m, s0.ID, asynctask.StatusSucceeded)
	got1, err := m.dbSrc.Fetch(ctx, "chain")
	if err != nil {
		t.Fatalf("s0 成功后 fetch s1: %v", err)
	}
	if got1.ID != s1.ID {
		t.Fatalf("应认领 s1(%s)，实际 %s", s1.ID, got1.ID)
	}
}

// TestChain_FailFastCascade 验证 FailFastChain=true 时，某 step 失败后同 chain 后续 step 置 canceled。
func TestChain_FailFastCascade(t *testing.T) {
	cfg := defaultConfig()
	cfg.FailFastChain = true
	m := newChainTestManager(t, cfg)
	ctx := context.Background()

	// MaxAttempts=1：首次失败即终态，无需在内存里伪造 attempt。
	s0, _, _ := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "chain", MaxAttempts: 1})
	s1, _, _ := m.SubmitAfter(ctx, s0.ID, &asynctask.AsyncTaskSpec{Type: "chain"})
	s2, _, _ := m.SubmitAfter(ctx, s1.ID, &asynctask.AsyncTaskSpec{Type: "chain"})

	// 走真实流程：先认领 s0（pending→running），再 Save 失败终态。
	claimed, err := m.dbSrc.Fetch(ctx, "chain")
	if err != nil || claimed.ID != s0.ID {
		t.Fatalf("fetch s0: id=%v err=%v", claimed, err)
	}
	err = m.dbSrc.Save(ctx, claimed, asynctask.Result{
		Outcome: asynctask.OutcomeFailed,
		Failure: asynctask.FailureHandler,
	}, nil)
	if err != nil {
		t.Fatalf("save s0 failed: %v", err)
	}

	// s1、s2 应被级联置 canceled。
	for _, id := range []string{s1.ID, s2.ID} {
		j, err := m.Get(ctx, id)
		if err != nil {
			t.Fatalf("get %s: %v", id, err)
		}
		if j.Status != asynctask.StatusCanceled {
			t.Fatalf("级联失败: asyncTask %s status=%s, 期望 canceled", id, j.Status)
		}
	}
}

// TestChain_FailFastOff 默认关闭时失败不级联。
func TestChain_FailFastOff(t *testing.T) {
	m := newChainTestManager(t, nil) // FailFastChain 默认 false
	ctx := context.Background()
	s0, _, _ := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "chain", MaxAttempts: 1})
	s1, _, _ := m.SubmitAfter(ctx, s0.ID, &asynctask.AsyncTaskSpec{Type: "chain"})

	claimed, err := m.dbSrc.Fetch(ctx, "chain")
	if err != nil {
		t.Fatalf("fetch s0: %v", err)
	}
	if err := m.dbSrc.Save(ctx, claimed, asynctask.Result{Outcome: asynctask.OutcomeFailed, Failure: asynctask.FailureHandler}, nil); err != nil {
		t.Fatalf("save s0: %v", err)
	}

	j, _ := m.Get(ctx, s1.ID)
	if j.Status != asynctask.StatusPending {
		t.Fatalf("FailFastChain 关闭时 s1 应仍 pending，实际 %s", j.Status)
	}
}
