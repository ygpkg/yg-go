package db

import (
	"testing"

	"github.com/ygpkg/yg-go/internal/testdb"
	"gorm.io/gorm"
)

// openTestDB 打开一个独立的 sqlite 内存库，并按 asynctask/ddl/sqlite.sql 建表。
//
// 建表走 DDL 而不是 AutoMigrate：sqlite 的部分唯一索引与 PG 同语义，
// 这样单测里的幂等语义才与生产一致（AutoMigrate 会把 uniq_key 建成全局唯一）。
func openTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	return testdb.OpenSQLite(t)
}
