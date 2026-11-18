// Package testdb 提供 asynctask 测试用的数据库构造：sqlite 内存库 / 真库 + 按方言执行
// asynctask/ddl 的建表语句。
//
// 只给测试用（内部包，不属于公开 API）：单测**不再走 AutoMigrate**，否则 C3 的
// 「活跃期唯一」会被 gorm tag 退化成「全局唯一」，把错误语义钉死在单测里。
package testdb

import (
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask/ddl"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// sqliteSeq 为每个测试库生成唯一名字，避免多个内存库共用 cache=shared 时互相污染。
var sqliteSeq int64

// 真库连接串：默认本机值，可用环境变量覆盖；连不上就 Skip（skip ≠ 验证过）。
const (
	defaultMySQLDSN    = "root:123456@tcp(127.0.0.1:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local&time_zone=%27%2B08%3A00%27"
	defaultPostgresDSN = "host=127.0.0.1 user=postgres password=123456 dbname=demo port=5432 sslmode=disable TimeZone=Asia/Shanghai"
)

// Open 打开一个已建好表（ddl 对应方言）的测试库。
//
// dialect 取 ddl.DialectPostgres / ddl.DialectMySQL / ddl.DialectSQLite：
//   - sqlite：独立的内存库（file:<唯一名>?mode=memory&cache=shared），每次调用都是干净库；
//   - postgres / mysql：按环境变量 POSTGRES_DSN / MYSQL_DSN（缺省本机值）连接，
//     连不上直接 t.Skip。真库**必须指向独占的测试库**：Open 会先整表重建（drop + ddl），
//     否则旧版本残留的表结构会让对账/断言误判。
//
// 返回的库已执行过 DDL，但**真库不保证空表**：需要隔离的用例应先调 Reset。
func Open(t testing.TB, dialect string) *gorm.DB {
	t.Helper()
	switch dialect {
	case ddl.DialectSQLite:
		return OpenSQLite(t)
	case ddl.DialectPostgres, ddl.DialectMySQL:
		return openReal(t, dialect)
	default:
		t.Fatalf("testdb.Open: 未知方言 %q", dialect)
		return nil
	}
}

// OpenSQLite 打开一个唯一的 sqlite 内存库并执行 ddl.SQLite()。
//
// 用 file:<唯一名>?mode=memory&cache=shared：裸 ":memory:" 会让连接池里每条连接各开一个
// 空库，gorm 下会读不到刚写入的行。
func OpenSQLite(t testing.TB) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:asynctask_test_%d?mode=memory&cache=shared", atomic.AddInt64(&sqliteSeq, 1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("testdb: open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, derr := db.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	})
	Apply(t, db)
	return db
}

// OpenSQLiteEmpty 打开一个唯一的 sqlite 内存库但**不建表**，用于验证「本包不执行 DDL」。
func OpenSQLiteEmpty(t testing.TB) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:asynctask_empty_%d?mode=memory&cache=shared", atomic.AddInt64(&sqliteSeq, 1))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("testdb: open sqlite: %v", err)
	}
	t.Cleanup(func() {
		if sqlDB, derr := db.DB(); derr == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// openReal 打开 PG / MySQL 真库（连不上则 Skip），并执行对应 DDL。
func openReal(t testing.TB, dialect string) *gorm.DB {
	t.Helper()
	var (
		dsn     string
		envName string
		openFn  func(string) gorm.Dialector
	)
	switch dialect {
	case ddl.DialectPostgres:
		envName, dsn, openFn = "POSTGRES_DSN", defaultPostgresDSN, postgres.Open
	case ddl.DialectMySQL:
		// 注意：DSN 的 loc 必须与会话 time_zone 一致（时间来源硬约束）。
		envName, dsn, openFn = "MYSQL_DSN", defaultMySQLDSN, mysql.Open
	}
	if v := os.Getenv(envName); v != "" {
		dsn = v
	}
	db, err := gorm.Open(openFn(dsn), &gorm.Config{Logger: gormlogger.Default.LogMode(gormlogger.Silent)})
	if err != nil {
		t.Skipf("skip %s: 连不上 (%v)", dialect, err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Skipf("skip %s: 取连接失败 (%v)", dialect, err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skipf("skip %s: ping 失败 (%v)", dialect, err)
	}
	// 真库是跨包共享的资源：go test ./... 会并行跑多个包，各自 Reset 同一批表。
	// 用文件锁把「同一方言的真库使用」串行化，否则会看到互相 DROP 导致的随机失败。
	// 先注册 close、后注册 release（Cleanup 是 LIFO）：关闭连接在释放锁之前完成。
	release := lockReal(t, dialect)
	t.Cleanup(release)
	t.Cleanup(func() { _ = sqlDB.Close() })
	Reset(t, db)
	return db
}

// lockReal 以独占文件锁串行化「同一方言真库」的跨包访问。
//
// 只用于测试：go test 并行跑多个包时，它们共用一个真库（demo），
// 各自 Reset 建表会互相踩踏（PG 报 pg_type 唯一键冲突、MySQL 报表不存在）。
// 文件锁是进程级、跨平台的；超时 120s，陈旧锁（>5min）自动清理。
func lockReal(t testing.TB, dialect string) func() {
	t.Helper()
	path := filepath.Join(os.TempDir(), "yg-go-asynctask-realdb-"+dialect+".lock")
	deadline := time.Now().Add(120 * time.Second)
	for {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			_, _ = fmt.Fprintf(f, "%d", os.Getpid())
			_ = f.Close()
			released := false
			return func() {
				if released {
					return
				}
				released = true
				_ = os.Remove(path)
			}
		}
		if !os.IsExist(err) {
			t.Fatalf("testdb: 建真库锁失败 %s: %v", path, err)
		}
		if st, serr := os.Stat(path); serr == nil && time.Since(st.ModTime()) > 5*time.Minute {
			_ = os.Remove(path) // 上一个进程崩溃留下的陈旧锁
			continue
		}
		if time.Now().After(deadline) {
			t.Fatalf("testdb: 等待真库锁超时（另一个测试包仍占用）: %s", path)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

// Apply 执行 db 对应方言的 ddl（幂等，可重复调用）。
func Apply(t testing.TB, db *gorm.DB) {
	t.Helper()
	if err := ddl.Apply(db); err != nil {
		t.Fatalf("testdb: apply ddl: %v", err)
	}
}

// Reset 把三张 asynctask 表整表重建（DROP + ddl.Apply），用于真库用例之间的隔离。
func Reset(t testing.TB, db *gorm.DB) {
	t.Helper()
	for _, table := range []string{
		"core_async_task_type",
		"core_async_task_run",
		"core_async_task",
	} {
		if err := db.Exec("DROP TABLE IF EXISTS " + table).Error; err != nil {
			t.Fatalf("testdb: drop %s: %v", table, err)
		}
	}
	Apply(t, db)
}

// Dialects 返回「sqlite + 可用真库」的方言列表，供跨方言语义用例遍历。
//
// 真库不可用时不返回该方言（调用方据此决定是否 skip 子测试）；sqlite 始终存在。
func Dialects() []string {
	out := []string{ddl.DialectSQLite}
	if os.Getenv("ASYNCTASK_SKIP_REALDB") != "" {
		return out
	}
	return append(out, ddl.DialectPostgres, ddl.DialectMySQL)
}
