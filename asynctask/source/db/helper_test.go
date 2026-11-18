package db

import (
	"fmt"
	"sync/atomic"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqliteSeq 为每个测试库生成唯一名字，避免多个内存库共用 cache=shared 时互相污染。
var sqliteSeq int64

// openTestDB 用 yg-go 现成的 gorm.io/driver/sqlite（mattn cgo）打开一个唯一的
// 共享内存库（file:<unique>?mode=memory&cache=shared），保证连接池内多条连接看到同一份数据
// （裸 ":memory:" 每条连接各开一个空库，gorm 连接池下会读不到刚写入的行）。
//
// 源测试用 ncruces 纯 Go 驱动；yg-go 用 mattn cgo 驱动替换，语义等价。
func openTestDB(t *testing.T, silent bool) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:asynctask_it_%d?mode=memory&cache=shared", atomic.AddInt64(&sqliteSeq, 1))
	cfg := &gorm.Config{}
	if silent {
		cfg.Logger = gormlogger.Default.LogMode(gormlogger.Silent)
	}
	db, err := gorm.Open(sqlite.Open(dsn), cfg)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	return db
}
