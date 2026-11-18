package manager

import (
	"strings"
	"testing"

	"github.com/ygpkg/yg-go/asynctask/ddl"
	"github.com/ygpkg/yg-go/internal/testdb"
)

// TestSchemaModels SchemaModels 只返回实体清单（三张表），且不包含任何 DDL 行为。
func TestSchemaModels(t *testing.T) {
	models := SchemaModels()
	if len(models) != 3 {
		t.Fatalf("SchemaModels len = %d, want 3", len(models))
	}
	// 清单里的表名必须出现在三份 DDL 里（只查是否出现，逐列对账在 asynctask/ddl 的测试里）。
	for dialect, text := range ddl.All() {
		for _, m := range models {
			name := tableNameOf(t, m)
			if !strings.Contains(text, "CREATE TABLE IF NOT EXISTS "+name) {
				t.Fatalf("[%s] DDL 缺少表 %s", dialect, name)
			}
		}
	}
}

// TestNewDoesNotCreateTable New 不再建表：空库构造成功、表不存在；执行 DDL 后才可用（且幂等）。
func TestNewDoesNotCreateTable(t *testing.T) {
	db := testdb.OpenSQLiteEmpty(t)

	// 1. 构造不自带建表：New 成功但表仍不存在。
	if _, err := New(nil, db); err != nil {
		t.Fatalf("New: %v", err)
	}
	for _, m := range SchemaModels() {
		if db.Migrator().HasTable(m) {
			t.Fatalf("New 不应建表，但表 %s 已存在", tableNameOf(t, m))
		}
	}

	// 2. 执行两次 DDL 幂等（第二次不报错），随后三张表都在。
	if err := ddl.Apply(db); err != nil {
		t.Fatalf("ddl.Apply #1: %v", err)
	}
	if err := ddl.Apply(db); err != nil {
		t.Fatalf("ddl.Apply #2 (idempotent): %v", err)
	}
	for _, m := range SchemaModels() {
		if !db.Migrator().HasTable(m) {
			t.Fatalf("执行 DDL 后表 %s 仍不存在", tableNameOf(t, m))
		}
	}
}
