package db

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// 直接对 finishAsyncTask 断言 RowsAffected 守卫：worker_id 不匹配时 UPDATE 匹配 0 行、不改写，
// 也不回填 task.Status。这是 DB 内部实现细节（finishAsyncTask 未导出），因此放在 source/db 内部测试。
func TestSource_LostLease_DirectFinishAsyncTaskRowsAffected(t *testing.T) {
	gdb := openTestDB(t)
	src, err := New(Config{DB: gdb, LeaseTTL: 60 * time.Second})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	ctx := context.Background()

	// 提交一条到点任务并经 Source 认领，得到 running 的 *asynctask.AsyncTask。
	now := time.Now()
	if err := gdb.Create(&CoreAsyncTask{
		ID: genUUID(), CreatedAt: now, UpdatedAt: now, TaskType: "directguard",
		Status: "pending", Generation: 1, MaxAttempts: 3, NextRunAt: now.Add(-time.Minute),
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	task, err := src.Fetch(ctx, "directguard")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	// 库里换 worker（模拟被接管）。
	if err := gdb.Exec(`UPDATE core_async_task SET worker_id = 'someone-else' WHERE id = ?`, task.ID).Error; err != nil {
		t.Fatalf("simulate takeover: %v", err)
	}

	stale := *task // 持有旧 worker_id 的任务视图
	before := stale.Status

	if err := src.finishAsyncTask(gdb, &stale, asynctask.StatusSucceeded, asynctask.Result{}, src.dialect.NowValueExpr()); err != nil {
		if !errors.Is(err, asynctask.ErrLeaseLost) {
			t.Fatalf("finishAsyncTask err = %v, want ErrLeaseLost", err)
		}
	}
	if stale.Status != before {
		t.Fatalf("task.Status mutated to %q, want unchanged %q (RowsAffected=0)", stale.Status, before)
	}
	var row CoreAsyncTask
	if err := gdb.Where("id = ?", task.ID).Take(&row).Error; err != nil {
		t.Fatalf("load asyncTask: %v", err)
	}
	if row.Status != string(asynctask.StatusRunning) || row.WorkerID != "someone-else" {
		t.Fatalf("db (status,worker) = (%q,%q), want (running, someone-else)", row.Status, row.WorkerID)
	}
}
