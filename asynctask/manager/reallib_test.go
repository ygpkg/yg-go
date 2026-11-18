package manager

import (
	"context"
	"testing"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/ddl"
	"github.com/ygpkg/yg-go/internal/testdb"
)

// TestRealC3ActiveWindow C3 的三条断言在 sqlite / PostgreSQL / MySQL 上各跑一遍。
//
// sqlite 也走 ddl.SQLite()（部分唯一索引），因此与 PG 同语义；真库连不上则跳过该方言。
func TestRealC3ActiveWindow(t *testing.T) {
	for _, dialect := range []string{ddl.DialectSQLite, ddl.DialectPostgres, ddl.DialectMySQL} {
		t.Run(dialect, func(t *testing.T) {
			db := testdb.Open(t, dialect) // 真库在 Open 里已整表重建；sqlite 每次都是新内存库
			m, err := New(nil, db)
			if err != nil {
				t.Fatalf("[%s] New: %v", dialect, err)
			}
			runC3Assertions(t, m)
		})
	}
}

// runC3Assertions 执行设计文档 §5.4 的三条断言：
//
//	Given 一条 failed 的 k1  When Submit(uniq_key=k1)  Then submitted=true 且返回新 id（不是旧 id）
//	Given 一条 pending 的 k1 When Submit(uniq_key=k1)  Then submitted=false 且返回同 id
//	Given 一条 running 的 k1 When 置 succeeded 后再 Submit(uniq_key=k1) Then submitted=true
func runC3Assertions(t *testing.T, m *Manager) {
	t.Helper()

	// 断言 1：终态（failed）后同键可重建。
	failed, ok := submitKeyed(t, m, "c3-failed")
	if !ok {
		t.Fatalf("首次提交应为新建")
	}
	forceStatus(t, m, failed.ID, asynctask.StatusFailed)
	reborn, ok := submitKeyed(t, m, "c3-failed")
	if !ok {
		t.Fatalf("failed 后同键重建：submitted=false，期望 true（旧任务 %s）", failed.ID)
	}
	if reborn.ID == failed.ID {
		t.Fatalf("failed 后同键重建返回了旧任务 %s", failed.ID)
	}
	if reborn.Status != asynctask.StatusPending {
		t.Fatalf("重建任务状态 = %q，期望 pending", reborn.Status)
	}

	// 断言 2：活跃期（pending）同键命中既有行。
	pending, _ := submitKeyed(t, m, "c3-pending")
	hit, ok := submitKeyed(t, m, "c3-pending")
	if ok {
		t.Fatalf("pending 期同键提交：submitted=true，期望 false")
	}
	if hit.ID != pending.ID {
		t.Fatalf("pending 期同键提交返回 %s，期望既有 %s", hit.ID, pending.ID)
	}

	// 断言 3：running → succeeded（终态）后同键可重建。
	running, _ := submitKeyed(t, m, "c3-running")
	forceStatus(t, m, running.ID, asynctask.StatusRunning)
	forceStatus(t, m, running.ID, asynctask.StatusSucceeded)
	again, ok := submitKeyed(t, m, "c3-running")
	if !ok {
		t.Fatalf("succeeded 后同键重建：submitted=false，期望 true")
	}
	if again.ID == running.ID {
		t.Fatalf("succeeded 后同键重建返回了旧任务 %s", running.ID)
	}

	// 旧回查口径的反向断言：GetByUniqKey 只认活跃行。
	if _, err := m.GetByUniqKey(context.Background(), "c3-failed"); err != nil {
		t.Fatalf("重建后 GetByUniqKey 应命中活跃行: %v", err)
	}
}
