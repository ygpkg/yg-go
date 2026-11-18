package db

import (
	"context"
	"testing"
	"time"
)

// newTestStore 用 gorm.io/driver/sqlite（mattn cgo）开一个内存库并 AutoMigrate 两表，返回 store。
//
// SQLite 无 SKIP LOCKED、无隔离级别设置：认领在这里退化为无锁普通事务，
// 只验证「认领成功 / 空队列 / 次数用尽 / 租约未过期」这类单连接语义，不验证并发。
func newTestStore(t *testing.T) *store {
	t.Helper()
	db := openTestDB(t, false)
	if err := db.AutoMigrate(&CoreAsyncTask{}, &CoreAsyncTaskRun{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	return newStore(db, DialectSQLite, 60*time.Second)
}

// insertAsyncTask 直接插入一行 core_async_task，字段按用例覆盖。
func insertAsyncTask(t *testing.T, s *store, j *CoreAsyncTask) string {
	t.Helper()
	if j.ID == "" {
		j.ID = newID()
	}
	if j.Status == "" {
		j.Status = "pending"
	}
	if j.Generation == 0 {
		j.Generation = 1
	}
	if j.MaxAttempts == 0 {
		j.MaxAttempts = 3
	}
	if j.CreatedAt.IsZero() {
		j.CreatedAt = time.Now()
	}
	if j.UpdatedAt.IsZero() {
		j.UpdatedAt = time.Now()
	}
	if err := s.db.Create(j).Error; err != nil {
		t.Fatalf("insert asyncTask: %v", err)
	}
	return j.ID
}

// TestClaimSuccess 认领一条到点的 pending 任务：status→running、attempt+1、worker 写入。
func TestClaimSuccess(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	id := insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType:  "invoice",
		Status:    "pending",
		NextRunAt: time.Now().Add(-time.Minute),
	})

	rows, err := s.claimBatch(ctx, "invoice", "w1", 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("claimed = %+v, want 1 row %s", rows, id)
	}
	if rows[0].Attempt != 1 {
		t.Fatalf("claimed attempt = %d, want 1", rows[0].Attempt)
	}

	var got CoreAsyncTask
	if err := s.db.Where("id = ?", id).Take(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Status != "running" {
		t.Errorf("status = %q, want running", got.Status)
	}
	if got.Attempt != 1 {
		t.Errorf("attempt = %d, want 1", got.Attempt)
	}
	if got.WorkerID != "w1" {
		t.Errorf("worker_id = %q, want w1", got.WorkerID)
	}
	if got.LeaseUntil == nil || !got.LeaseUntil.After(time.Now()) {
		t.Errorf("lease_until = %v, want future", got.LeaseUntil)
	}
	if got.StartedAt == nil {
		t.Errorf("started_at nil, want set")
	}
}

// TestClaimEmpty 空队列认领返回空切片。
func TestClaimEmpty(t *testing.T) {
	s := newTestStore(t)
	rows, err := s.claimBatch(context.Background(), "invoice", "w1", 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("claimed = %+v, want empty", rows)
	}
}

// TestClaimAttemptsExhausted 次数已用尽（attempt>=max_attempts）即使到点也不认领。
func TestClaimAttemptsExhausted(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType:    "invoice",
		Status:      "pending",
		NextRunAt:   time.Now().Add(-time.Minute),
		Attempt:     3,
		MaxAttempts: 3,
	})
	rows, err := s.claimBatch(ctx, "invoice", "w1", 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("claimed = %+v, want none (attempts exhausted)", rows)
	}
}

// TestClaimFutureNotEligible next_run_at 未到点不认领。
func TestClaimFutureNotEligible(t *testing.T) {
	s := newTestStore(t)
	insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType:  "invoice",
		Status:    "pending",
		NextRunAt: time.Now().Add(time.Hour),
	})
	rows, err := s.claimBatch(context.Background(), "invoice", "w1", 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("claimed = %+v, want none (future)", rows)
	}
}

// TestClaimLeaseNotExpired 租约未过期的 running 任务不被抢占。
func TestClaimLeaseNotExpired(t *testing.T) {
	s := newTestStore(t)
	lease := time.Now().Add(time.Minute)
	insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType:   "invoice",
		Status:     "running",
		NextRunAt:  time.Now().Add(-time.Minute),
		Attempt:    1,
		LeaseUntil: &lease,
		WorkerID:   "other",
	})
	rows, err := s.claimBatch(context.Background(), "invoice", "w1", 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("claimed = %+v, want none (lease valid)", rows)
	}
}

// TestClaimTakeOverExpiredLease 租约过期的 running 任务被接管，attempt+1。
func TestClaimTakeOverExpiredLease(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Minute)
	id := insertAsyncTask(t, s, &CoreAsyncTask{
		TaskType:   "invoice",
		Status:     "running",
		NextRunAt:  time.Now().Add(-time.Minute),
		Attempt:    1,
		LeaseUntil: &past,
		WorkerID:   "dead",
	})
	rows, err := s.claimBatch(ctx, "invoice", "w2", 10)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != id {
		t.Fatalf("claimed = %+v, want takeover of %s", rows, id)
	}
	var got CoreAsyncTask
	if err := s.db.Where("id = ?", id).Take(&got).Error; err != nil {
		t.Fatal(err)
	}
	if got.Attempt != 2 {
		t.Errorf("attempt = %d, want 2 (takeover +1)", got.Attempt)
	}
	if got.WorkerID != "w2" {
		t.Errorf("worker_id = %q, want w2", got.WorkerID)
	}
}

// TestClaimPriorityOrder 高优先级先认领。
func TestClaimPriorityOrder(t *testing.T) {
	s := newTestStore(t)
	base := time.Now().Add(-time.Minute)
	low := insertAsyncTask(t, s, &CoreAsyncTask{TaskType: "invoice", Status: "pending", NextRunAt: base, Priority: 0})
	high := insertAsyncTask(t, s, &CoreAsyncTask{TaskType: "invoice", Status: "pending", NextRunAt: base, Priority: 10})
	rows, err := s.claimBatch(context.Background(), "invoice", "w1", 1)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("claimed = %d, want 1", len(rows))
	}
	if rows[0].ID != high {
		t.Errorf("claimed %s, want high-priority %s (low=%s)", rows[0].ID, high, low)
	}
}

// newID 生成 UUID v4 风格 ID（测试用，简化实现）。
func newID() string { return genUUID() }
