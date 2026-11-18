// Package ddl 提供 asynctask 的建表语句（PostgreSQL / MySQL / SQLite）。
//
// 本包**不执行任何 DDL**：真源从 gorm tag 挪到可评审的 SQL 文件，库由应用的统一迁移流程
// 执行一次（见 ddl.Apply 仅供测试/示例使用）。三份文件都是幂等的，可重复重放。
//
// 为什么是三份而不是一份：
//   - MySQL 没有 CREATE INDEX IF NOT EXISTS → 索引必须内联进 CREATE TABLE；
//   - MySQL 没有部分唯一索引 → 用 STORED 生成列 uniq_key_active 模拟「活跃期唯一」；
//   - SQLite 支持部分唯一索引（与 PG 同语义），因此单测与生产语义一致。
package ddl

import (
	"embed"
	"errors"
	"fmt"
	"strings"
	"sync"

	"gorm.io/gorm"
)

//go:embed postgres.sql mysql.sql sqlite.sql
var files embed.FS

// 方言短名，与 All() 的 map key、source/db.DetectDialect 的判定口径一致。
const (
	DialectPostgres = "postgres"
	DialectMySQL    = "mysql"
	DialectSQLite   = "sqlite"
)

// 哨兵错误。
var (
	// ErrUnsupportedDialect 方言不在支持列表（仅 postgres / mysql / sqlite）。
	ErrUnsupportedDialect = errors.New("asynctask/ddl: unsupported dialect")
	// ErrNilDB db 为 nil。
	ErrNilDB = errors.New("asynctask/ddl: db is nil")
)

var (
	readOnce sync.Once
	texts    = map[string]string{}
	errRead  error
)

// load 一次性读入三份 SQL（//go:embed 内容只读，进程内缓存）。
func load() (map[string]string, error) {
	readOnce.Do(func() {
		for name, file := range map[string]string{
			DialectPostgres: "postgres.sql",
			DialectMySQL:    "mysql.sql",
			DialectSQLite:   "sqlite.sql",
		} {
			b, err := files.ReadFile(file)
			if err != nil {
				errRead = fmt.Errorf("asynctask/ddl: read %s: %w", file, err)
				return
			}
			texts[name] = string(b)
		}
	})
	return texts, errRead
}

// Postgres 返回 PostgreSQL 12+ 的完整 DDL（幂等）。
func Postgres() string { return mustText(DialectPostgres) }

// MySQL 返回 MySQL 8.0.1+ 的完整 DDL（幂等；含生成列 uniq_key_active）。
func MySQL() string { return mustText(DialectMySQL) }

// SQLite 返回 SQLite 3 的完整 DDL（仅测试用；与 PG 同语义）。
func SQLite() string { return mustText(DialectSQLite) }

// All 返回方言 → DDL 全文的映射。
func All() map[string]string {
	texts, err := load()
	if err != nil {
		panic(err)
	}
	out := make(map[string]string, len(texts))
	for k, v := range texts {
		out[k] = v
	}
	return out
}

// Statements 把某方言的 DDL 拆成可逐条执行的语句（去注释、去空行、去掉行尾分号）。
//
// 拆分依据：DDL 里的分号只作为语句分隔符出现（字符串字面量、生成列表达式内都没有分号），
// 因此按 ";" 切分是安全的；注释行（-- 开头）先整行丢弃。
func Statements(dialect string) ([]string, error) {
	text := mustText(dialect)
	if text == "" {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedDialect, dialect)
	}
	lines := strings.Split(text, "\n")
	var body strings.Builder
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "--") {
			continue
		}
		body.WriteString(line)
		body.WriteString("\n")
	}
	var out []string
	for _, raw := range strings.Split(body.String(), ";") {
		stmt := strings.TrimSpace(raw)
		if stmt == "" {
			continue
		}
		out = append(out, stmt)
	}
	return out, nil
}

// Apply 按 db 的方言执行对应 DDL（逐条 Exec）。
//
// 仅用于测试与示例：生产环境的 DDL 应由统一迁移流程执行一次，服务侧不应各自建表。
// gorm 的 dialector.Name() 返回值与方言名的映射：mysql / postgres / sqlite。
func Apply(db *gorm.DB) error {
	if db == nil {
		return ErrNilDB
	}
	dialect := dialectOf(db.Dialector.Name())
	stmts, err := Statements(dialect)
	if err != nil {
		return err
	}
	for _, stmt := range stmts {
		if err := db.Exec(stmt).Error; err != nil {
			// 只保留语句的前 80 个字符：整段 DDL 出错时日志会非常长。
			return fmt.Errorf("asynctask/ddl: apply %s: %w (statement: %.80s…)", dialect, err, stmt)
		}
	}
	return nil
}

// dialectOf 把 gorm 的 dialector 名归一到本包的方言短名。
func dialectOf(name string) string {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mysql":
		return DialectMySQL
	case "postgres", "postgresql":
		return DialectPostgres
	case "sqlite", "sqlite3":
		return DialectSQLite
	default:
		return name
	}
}

// mustText 取某方言 DDL 全文；未知方言返回空串（由调用方决定如何处理）。
func mustText(dialect string) string {
	texts, err := load()
	if err != nil {
		panic(err)
	}
	return texts[dialect]
}
