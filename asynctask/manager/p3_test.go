package manager

import (
	"context"
	"testing"

	"github.com/ygpkg/yg-go/asynctask"
)

// TestP3_LeaseRegistry 验证默认租约推导判活：认领中的 worker 视为在线，过期后不在线。
func TestP3_LeaseRegistry(t *testing.T) {
	m := newChainTestManager(t, nil)
	ctx := context.Background()

	// 提交并认领一条任务（认领会写 worker_id + lease_until）。
	_, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "t"})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	claimed, err := m.dbSrc.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}

	reg := m.Registry()
	alive, err := reg.IsWorkerAlive(ctx, "t", claimed.WorkerID)
	if err != nil {
		t.Fatalf("IsWorkerAlive: %v", err)
	}
	if !alive {
		t.Fatalf("认领中的 worker 应在线")
	}
	n, err := reg.AliveWorkers(ctx, "t")
	if err != nil {
		t.Fatalf("AliveWorkers: %v", err)
	}
	if n != 1 {
		t.Fatalf("在线 worker 数=%d, 期望 1", n)
	}

	// 手工把租约置为过期（相对当前 -1 分钟，用 SQL 表达式避免时区/格式差异）→ 不再在线。
	if err := m.db.Exec("UPDATE core_async_task SET lease_until = datetime('now','-1 minute') WHERE id = ?", claimed.ID).Error; err != nil {
		t.Fatalf("expire lease: %v", err)
	}
	alive2, _ := reg.IsWorkerAlive(ctx, "t", claimed.WorkerID)
	if alive2 {
		t.Fatalf("租约过期后不应在线")
	}
	n2, _ := reg.AliveWorkers(ctx, "t")
	if n2 != 0 {
		t.Fatalf("过期后在线数=%d, 期望 0", n2)
	}
}
