package manager

// 本期不带 asynq/redis 子包。
//
// 源 golib 的 notifier_test.go / p3_test.go / p4_test.go 中依赖
// github.com/hibiken/asynq、github.com/alicebob/miniredis/v2 或 manager/notifier/asynq
// 子包的用例不移植（本期不引入这些依赖）。纯逻辑用例（NotifierQueue/NopNotifier、
// 租约判活、List 过滤/排序/时间区间、超时状态等）已在 notifier_pure_test.go /
// p3_test.go / p4_test.go 中单独移植。

import (
	"testing"

	"github.com/ygpkg/yg-go/internal/testdb"
	"gorm.io/gorm"
)

// openTestDB 打开一个独立的 sqlite 内存库，并按 asynctask/ddl/sqlite.sql 建表。
//
// 建表走 DDL 而不是 AutoMigrate：uniq_key 的唯一性只在活跃期生效，
// AutoMigrate 会把它建成全局唯一，让单测锁死错误语义。
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.OpenSQLite(t)
}

// newChainTestManager 构造一个可达内部字段的 Manager（用于直接驱动 source/db 认领）。
func newChainTestManager(t *testing.T, cfg *Config) *Manager {
	t.Helper()
	gdb := openTestDB(t)
	m, err := New(cfg, gdb)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return m
}
