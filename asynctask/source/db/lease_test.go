package db

import (
	"context"
	"testing"
	"time"
)

// TestRenewOK 续租成功：仍持租且 running，返回 RenewOK 并延后 lease_until。
func TestRenewOK(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Minute)
	id := insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType: "invoice", Status: "running", Attempt: 1,
		NextRunAt: time.Now().Add(-time.Minute), LeaseUntil: &past, WorkerID: "w1",
	})
	res, err := s.renew(ctx, id, "w1")
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if res != RenewOK {
		t.Fatalf("renew result = %v, want RenewOK", res)
	}
	var got CoreAsyncTask
	if err := s.db.Where("id = ?", id).Take(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.LeaseUntil == nil || !got.LeaseUntil.After(time.Now().Add(30*time.Second)) {
		t.Errorf("lease_until = %v, want extended", got.LeaseUntil)
	}
}

// TestRenewLostToOtherWorker 被别的 worker 持有时续租失败 → RenewLeaseExpired。
func TestRenewLostToOtherWorker(t *testing.T) {
	s := newTestStore(t)
	future := time.Now().Add(time.Minute)
	id := insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType: "invoice", Status: "running", Attempt: 1,
		NextRunAt: time.Now().Add(-time.Minute), LeaseUntil: &future, WorkerID: "w2",
	})
	res, err := s.renew(context.Background(), id, "w1")
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if res != RenewLeaseExpired {
		t.Fatalf("renew result = %v, want RenewLeaseExpired", res)
	}
}

// TestRenewCanceled 已 canceled 时续租失败 → RenewCanceled。
func TestRenewCanceled(t *testing.T) {
	s := newTestStore(t)
	id := insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType: "invoice", Status: "canceled", Attempt: 1,
		NextRunAt: time.Now().Add(-time.Minute), WorkerID: "w1",
	})
	res, err := s.renew(context.Background(), id, "w1")
	if err != nil {
		t.Fatalf("renew: %v", err)
	}
	if res != RenewCanceled {
		t.Fatalf("renew result = %v, want RenewCanceled", res)
	}
}

// TestReapStaleRunLongRunningNotReaped 长任务（>2×LeaseTTL）仍在正常续租不被误判残留。
func TestReapStaleRunLongRunningNotReaped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	// 任务 lease 仍未来（正常续租），asyncTask attempt==run attempt。
	future := time.Now().Add(time.Minute)
	asyncTaskID := insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType: "invoice", Status: "running", Attempt: 1,
		NextRunAt: time.Now().Add(-time.Hour), LeaseUntil: &future, WorkerID: "w1",
		Generation: 1,
	})
	started := time.Now().Add(-3 * time.Minute) // 长任务
	insertRun(t, s, &CoreAsyncTaskRun{
		AsyncTaskID: asyncTaskID, Generation: 1, Attempt: 1, Status: "running",
		WorkerID: "w1", StartedAt: started,
	})
	runs, asyncTasks, err := s.reap(ctx, 100)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if runs != 0 || asyncTasks != 0 {
		t.Fatalf("reap runs=%d asyncTasks=%d, want 0/0 (long running must survive)", runs, asyncTasks)
	}
}

// TestReapStaleRunReaped asyncTask 已终态但尝试行仍 running → 收口为 failed/lease_expired。
func TestReapStaleRunReaped(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	// asyncTask 已 failed（不再 running），尝试行残留 running。
	pid := insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType: "invoice", Status: "failed", Attempt: 1,
		NextRunAt: time.Now().Add(-time.Hour), Generation: 1, WorkerID: "",
	})
	insertRun(t, s, &CoreAsyncTaskRun{
		AsyncTaskID: pid, Generation: 1, Attempt: 1, Status: "running",
		WorkerID: "w1", StartedAt: time.Now().Add(-time.Minute),
	})
	runs, _, err := s.reap(ctx, 100)
	if err != nil {
		t.Fatalf("reap: %v", err)
	}
	if runs != 1 {
		t.Fatalf("reap runs=%d, want 1", runs)
	}
	var r CoreAsyncTaskRun
	if err := s.db.Where("async_task_id = ?", pid).Take(&r).Error; err != nil {
		t.Fatal(err)
	}
	if r.Status != "failed" || r.FailureKind != "lease_expired" {
		t.Errorf("run status=%q failure=%q, want failed/lease_expired", r.Status, r.FailureKind)
	}
	if r.FinishedAt == nil {
		t.Errorf("finished_at nil, want set")
	}
}

// TestReapIdempotent 回收器连跑两次结果一致（幂等）。
func TestReapIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	// 一个次数用尽且租约过期的 running 任务 → 判死。
	past := time.Now().Add(-time.Minute)
	deadID := insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType: "invoice", Status: "running", Attempt: 3, FailureCount: 3, MaxAttempts: 3,
		NextRunAt: time.Now().Add(-time.Hour), LeaseUntil: &past, WorkerID: "w1",
	})
	runs1, tasks1, err := s.reap(ctx, 100)
	if err != nil {
		t.Fatalf("reap#1: %v", err)
	}
	runs2, tasks2, err := s.reap(ctx, 100)
	if err != nil {
		t.Fatalf("reap#2: %v", err)
	}
	if tasks1 != 1 {
		t.Fatalf("reap#1 asyncTasks=%d, want 1", tasks1)
	}
	if runs2 != 0 || tasks2 != 0 {
		t.Fatalf("reap#2 runs=%d asyncTasks=%d, want 0/0 (idempotent)", runs2, tasks2)
	}
	var got CoreAsyncTask
	if err := s.db.Where("id = ?", deadID).Take(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != "failed" {
		t.Errorf("status = %q, want failed", got.Status)
	}
	if got.WorkerID != "" || got.LeaseUntil != nil {
		t.Errorf("worker_id=%q lease=%v, want cleared", got.WorkerID, got.LeaseUntil)
	}
	if got.FinishedAt == nil {
		t.Errorf("finished_at nil, want set")
	}
	_ = runs1
}

// insertRun 插入一行 core_async_task_run。
func insertRun(t *testing.T, s *store, r *CoreAsyncTaskRun) string {
	t.Helper()
	if r.ID == "" {
		r.ID = genUUID()
	}
	if r.CreatedAt.IsZero() {
		r.CreatedAt = time.Now()
	}
	if err := s.db.Create(r).Error; err != nil {
		t.Fatalf("insert run: %v", err)
	}
	return r.ID
}
