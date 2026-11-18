package manager

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"
)

// newTestManager 用 sqlite 内存库（已按 ddl.SQLite() 建表）构造 Manager。
func newTestManager(t *testing.T) *Manager {
	t.Helper()
	m, err := New(nil, openTestDB(t))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}

// TestNewNilDB db==nil → ErrNilDB。
func TestNewNilDB(t *testing.T) {
	if _, err := New(nil, nil); err != asynctask.ErrNilDB {
		t.Fatalf("New(nil db) err = %v, want ErrNilDB", err)
	}
}

// TestNewInvalidConfig PollInterval<100ms → ErrInvalidConfig。
func TestNewInvalidConfig(t *testing.T) {
	cfg := defaultConfig()
	cfg.PollInterval = 50 * time.Millisecond
	if _, err := New(cfg, openTestDB(t)); !errors.Is(err, asynctask.ErrInvalidConfig) {
		t.Fatalf("New(invalid) err = %v, want ErrInvalidConfig", err)
	}
}

// TestSubmitBasic 基本提交：submitted=true。
func TestSubmitBasic(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	asyncTask, submitted, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice", Payload: []byte(`{"a":1}`)})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if !submitted || asyncTask == nil {
		t.Fatalf("submitted=%v asyncTask=%v, want true + asyncTask", submitted, asyncTask)
	}
	if asyncTask.Status != asynctask.StatusPending {
		t.Errorf("status = %q, want pending", asyncTask.Status)
	}
	if asyncTask.Generation != 1 || asyncTask.Attempt != 0 {
		t.Errorf("gen=%d attempt=%d, want 1/0", asyncTask.Generation, asyncTask.Attempt)
	}
	if asyncTask.MaxAttempts != 3 {
		t.Errorf("max_attempts = %d, want default 3", asyncTask.MaxAttempts)
	}
}

// TestSubmitEmptyType ErrEmptyType。
func TestSubmitEmptyType(t *testing.T) {
	m := newTestManager(t)
	if _, _, err := m.Submit(context.Background(), &asynctask.AsyncTaskSpec{}); err != asynctask.ErrEmptyType {
		t.Fatalf("err = %v, want ErrEmptyType", err)
	}
}

// TestSubmitPayloadTooLarge 超 MaxPayloadBytes → ErrPayloadTooLarge。
func TestSubmitPayloadTooLarge(t *testing.T) {
	m := newTestManager(t)
	_, _, err := m.Submit(context.Background(), &asynctask.AsyncTaskSpec{
		Type: "invoice", Payload: make([]byte, m.cfg.MaxPayloadBytes+1),
	})
	if err != asynctask.ErrPayloadTooLarge {
		t.Fatalf("err = %v, want ErrPayloadTooLarge", err)
	}
}

// TestSubmitIdempotentUniqKey 同键第二次提交 submitted=false，返回既有任务。
func TestSubmitIdempotentUniqKey(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	j1, s1, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice", UniqKey: "k1"})
	if err != nil || !s1 {
		t.Fatalf("first submit: s=%v err=%v", s1, err)
	}
	j2, s2, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice", UniqKey: "k1"})
	if err != nil {
		t.Fatalf("second submit: %v", err)
	}
	if s2 {
		t.Fatalf("second submitted=true, want false (idempotent hit)")
	}
	if j2.ID != j1.ID {
		t.Errorf("j2.ID = %s, want existing %s", j2.ID, j1.ID)
	}
}

// TestA3SameKeyConcurrent 同键并发 100 次只落 1 行。
func TestA3SameKeyConcurrent(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	const n = 100
	var wg sync.WaitGroup
	var mu sync.Mutex
	created := 0
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, submitted, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice", UniqKey: "same-key"})
			if err != nil {
				// sqlite 单写者可能 busy；允许重试后成功。
				_, submitted, err = m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice", UniqKey: "same-key"})
				if err != nil {
					return
				}
			}
			if submitted {
				mu.Lock()
				created++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	var cnt int64
	if err := m.db.Model(&sourcedb.CoreAsyncTask{}).Where("uniq_key = ?", "same-key").Count(&cnt).Error; err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("rows = %d, want 1 (A3 same-key)", cnt)
	}
	if created != 1 {
		t.Fatalf("submitted=true count = %d, want 1", created)
	}
}

// TestA3NoKeyTwoRows 两条无键任务落 2 行且都 submitted=true。
func TestA3NoKeyTwoRows(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	_, s1, err1 := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	_, s2, err2 := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	if err1 != nil || err2 != nil {
		t.Fatalf("errs = %v %v", err1, err2)
	}
	if !s1 || !s2 {
		t.Fatalf("submitted = %v %v, want both true", s1, s2)
	}
	var cnt int64
	m.db.Model(&sourcedb.CoreAsyncTask{}).Count(&cnt)
	if cnt != 2 {
		t.Fatalf("rows = %d, want 2 (empty uniq_key → NULL, not deduped)", cnt)
	}
}

// TestCancelPendingFlips pending 立即翻转 canceled。
func TestCancelPendingFlips(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	asyncTask, _, _ := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	canceled, err := m.Cancel(ctx, asyncTask.ID)
	if err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if !canceled {
		t.Fatalf("canceled = false, want true")
	}
	got, _ := m.Get(ctx, asyncTask.ID)
	if got.Status != asynctask.StatusCanceled {
		t.Errorf("status = %q, want canceled", got.Status)
	}
}

// TestCancelTerminalIdempotent 终态 Cancel 幂等返回 (false, nil)。
func TestCancelTerminalIdempotent(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	asyncTask, _, _ := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	if _, err := m.Cancel(ctx, asyncTask.ID); err != nil {
		t.Fatalf("first cancel: %v", err)
	}
	canceled, err := m.Cancel(ctx, asyncTask.ID)
	if err != nil {
		t.Fatalf("second cancel: %v", err)
	}
	if canceled {
		t.Fatalf("second cancel canceled=true, want false (idempotent)")
	}
}

// TestCancelNotFound 不存在 → ErrAsyncTaskNotFound。
func TestCancelNotFound(t *testing.T) {
	m := newTestManager(t)
	if _, err := m.Cancel(context.Background(), "nope"); err != asynctask.ErrAsyncTaskNotFound {
		t.Fatalf("err = %v, want ErrAsyncTaskNotFound", err)
	}
}

// TestReplayTerminal 终态 Replay → generation+1、回到 pending。
func TestReplayTerminal(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	asyncTask, _, _ := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	// 做成功终态。
	m.db.Model(&sourcedb.CoreAsyncTask{}).Where("id = ?", asyncTask.ID).Updates(map[string]any{
		"status": "succeeded", "attempt": 1, "finished_at": time.Now(),
	})
	rp, err := m.Replay(ctx, asyncTask.ID)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if rp.Status != asynctask.StatusPending {
		t.Errorf("status = %q, want pending", rp.Status)
	}
	if rp.Generation != 2 {
		t.Errorf("generation = %d, want 2", rp.Generation)
	}
	if rp.Attempt != 0 {
		t.Errorf("attempt = %d, want 0", rp.Attempt)
	}
}

// TestReplayNonTerminal 非终态 Replay → ErrNotReplayable。
func TestReplayNonTerminal(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	asyncTask, _, _ := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"}) // pending
	if _, err := m.Replay(ctx, asyncTask.ID); err != asynctask.ErrNotReplayable {
		t.Fatalf("Replay(pending) err = %v, want ErrNotReplayable", err)
	}
}

// TestStats 统计各状态计数。
func TestStats(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	j2, _, _ := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	m.Cancel(ctx, j2.ID)
	st, err := m.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if st.Pending != 1 || st.Canceled != 1 || st.Total != 2 {
		t.Fatalf("stats = %+v, want pending1 canceled1 total2", st)
	}
}

// TestListPagination List 分页返回 total。
func TestListPagination(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "invoice"})
	}
	asyncTasks, total, err := m.List(ctx, &asynctask.AsyncTaskCond{Page: 1, PageSize: 2})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 5 {
		t.Errorf("total = %d, want 5", total)
	}
	if len(asyncTasks) != 2 {
		t.Errorf("len = %d, want 2", len(asyncTasks))
	}
}

// TestMode 无通知器为 poll。
func TestMode(t *testing.T) {
	m := newTestManager(t)
	if m.Mode() != ModePoll {
		t.Fatalf("mode = %v, want poll", m.Mode())
	}
}

// TestCloseIdempotent Close 幂等返回 nil。
func TestCloseIdempotent(t *testing.T) {
	m := newTestManager(t)
	if err := m.Close(); err != nil {
		t.Fatalf("Close#1: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close#2: %v", err)
	}
}

// TestStoreReturnsSatisfiesInterface Store() 返回可用 Store。
func TestStoreReturns(t *testing.T) {
	m := newTestManager(t)
	if m.Store() == nil {
		t.Fatal("Store() nil")
	}
}
