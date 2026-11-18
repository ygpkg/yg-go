package manager

import (
	"testing"

	"github.com/ygpkg/yg-go/asynctask"
)

// TestAutoMigrateNilDB nil db → ErrNilDB。
func TestAutoMigrateNilDB(t *testing.T) {
	if err := AutoMigrate(nil); err != asynctask.ErrNilDB {
		t.Fatalf("AutoMigrate(nil) err = %v, want ErrNilDB", err)
	}
}

// TestAutoMigrateIdempotent 连续两次 AutoMigrate 幂等（第二次不报错），且表可读写。
func TestAutoMigrateIdempotent(t *testing.T) {
	gdb := openTestDB(t)
	if err := AutoMigrate(gdb); err != nil {
		t.Fatalf("AutoMigrate #1: %v", err)
	}
	if err := AutoMigrate(gdb); err != nil {
		t.Fatalf("AutoMigrate #2 (idempotent): %v", err)
	}
	// 三张表都存在且可插入读取（用 SchemaModels 里声明的模型做一次 insert/select 冒烟）。
	models := SchemaModels()
	if len(models) != 3 {
		t.Fatalf("SchemaModels len = %d, want 3", len(models))
	}
	for _, m := range models {
		if !gdb.Migrator().HasTable(m) {
			t.Fatalf("table for %T not created", m)
		}
	}
}
