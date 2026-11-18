package db

import (
	"context"
	"testing"
	"time"
)

// TestCleanupDeletesTerminalOld 清理删除终态且 finished_at 早于保留期的任务与其尝试行。
func TestCleanupDeletesTerminalOld(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	old := time.Now().Add(-48 * time.Hour)
	// 终态、finished_at 早于 before。
	id := insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType: "invoice", Status: "succeeded", Attempt: 1,
		NextRunAt: old, FinishedAt: &old,
	})
	insertRun(t, s, &CoreAsyncTaskRun{
		AsyncTaskID: id, Generation: 1, Attempt: 1, Status: "succeeded",
		StartedAt: old, FinishedAt: &old,
	})

	before := time.Now().Add(-24 * time.Hour) // 保留 24h
	tasks, runs, err := s.Cleanup(ctx, before, 200)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if tasks != 1 || runs != 1 {
		t.Fatalf("cleanup tasks=%d runs=%d, want 1/1", tasks, runs)
	}
	if n, _ := s.countAsyncTasks(ctx); n != 0 {
		t.Errorf("asyncTasks left = %d, want 0", n)
	}
	if n, _ := s.countRuns(ctx); n != 0 {
		t.Errorf("runs left = %d, want 0", n)
	}
}

// TestCleanupKeepsPendingRunning pending/running 永不删。
func TestCleanupKeepsPendingRunning(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	old := time.Now().Add(-48 * time.Hour)
	insertAsyncTask(t, s, &CoreAsyncTask{TaskType: "invoice", Status: "pending", NextRunAt: old})
	insertAsyncTask(t, s, &CoreAsyncTask{TaskType: "invoice", Status: "running", NextRunAt: old, Attempt: 1})
	// 终态但 finished_at 不到期。
	recent := time.Now().Add(-time.Hour)
	insertAsyncTask(t, s, &CoreAsyncTask{TaskType: "invoice", Status: "failed", NextRunAt: old, FinishedAt: &recent})

	before := time.Now().Add(-24 * time.Hour)
	tasks, runs, err := s.Cleanup(ctx, before, 200)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if tasks != 0 || runs != 0 {
		t.Fatalf("cleanup tasks=%d runs=%d, want 0/0 (pending/running/recent kept)", tasks, runs)
	}
	if n, _ := s.countAsyncTasks(ctx); n != 3 {
		t.Errorf("asyncTasks left = %d, want 3", n)
	}
}

// TestCleanupBatch 单批只删 batch 条。
func TestCleanupBatch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	old := time.Now().Add(-48 * time.Hour)
	for i := 0; i < 5; i++ {
		insertAsyncTask(t, s, &CoreAsyncTask{TaskType: "invoice", Status: "failed", NextRunAt: old, FinishedAt: &old})
	}
	before := time.Now().Add(-24 * time.Hour)
	tasks, _, err := s.Cleanup(ctx, before, 2)
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if tasks != 5 {
		t.Fatalf("cleanup tasks=%d, want 5 (loops batches)", tasks)
	}
	if n, _ := s.countAsyncTasks(ctx); n != 0 {
		t.Errorf("asyncTasks left = %d, want 0", n)
	}
}

func (s *store) countAsyncTasks(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&CoreAsyncTask{}).Count(&n).Error
	return n, err
}

func (s *store) countRuns(ctx context.Context) (int64, error) {
	var n int64
	err := s.db.WithContext(ctx).Model(&CoreAsyncTaskRun{}).Count(&n).Error
	return n, err
}
