package manager

// 本期不带 asynq/redis 子包。
//
// 源 golib 的 notifier_test.go / p3_test.go / p4_test.go 中依赖
// github.com/hibiken/asynq、github.com/alicebob/miniredis/v2 或 manager/notifier/asynq
// 子包的用例不移植（本期不引入这些依赖）。纯逻辑用例（NotifierQueue/NopNotifier、
// 租约判活、List 过滤/排序/时间区间、超时状态等）已在 notifier_pure_test.go /
// p3_test.go / p4_test.go 中单独移植。

import (
	"fmt"
	"sync/atomic"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var testDBSeq int64

// openTestDB 用 yg-go 现成的 gorm.io/driver/sqlite（mattn cgo）开一个独立的内存库。
// 用 file:<唯一名>?mode=memory&cache=shared 保证每个测试拿到互不干扰的库。
// gorm 日志置 Silent，避免测试输出被 SQL 刷屏。
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:asynctask_mgr_%d?mode=memory&cache=shared", atomic.AddInt64(&testDBSeq, 1))
	gdb, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return gdb
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
