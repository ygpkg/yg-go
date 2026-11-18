// DDL ↔ 实体双向对账（C1 的核心价值）：实体（gorm 结构体）与 ddl/*.sql 的列名、索引名
// 必须双向一致，任何一侧多/少一个都红。
//
// 只比名字不比类型：方言类型差异（timestamptz / datetime(3) / varchar）噪声太大，
// 类型由真库实测兜底。逐方言白名单只有两处「DDL 独有」：
//   - uniq_key_active 列（仅 MySQL：模拟部分唯一索引的生成列，实体不映射）；
//   - uk_core_async_task_uniq_active 索引（三方言都有，但 gorm tag 不声明它——
//     它的谓词/生成列表达式无法用 tag 表达，只存在于 DDL）。
package ddl_test

import (
	"strings"
	"sync"
	"testing"

	"github.com/ygpkg/yg-go/asynctask/ddl"
	"github.com/ygpkg/yg-go/asynctask/manager"
	"github.com/ygpkg/yg-go/internal/testdb"
	"gorm.io/gorm/schema"
)

// activeUniqIndex 是 DDL 独有（gorm tag 无法表达）的唯一索引名。
const activeUniqIndex = "uk_core_async_task_uniq_active"

// ddlOnlyColumn 返回某方言某表允许「DDL 有、实体没有」的列。
func ddlOnlyColumn(dialect, table, column string) bool {
	return dialect == ddl.DialectMySQL && column == "uniq_key_active"
}

// ddlOnlyIndex 返回某方言某表允许「DDL 有、实体没有」的索引。
func ddlOnlyIndex(dialect, table, index string) bool {
	_ = dialect // 三方言都有这条部分唯一/生成列唯一索引
	return table == "core_async_task" && index == activeUniqIndex
}

// TestDDLMatchesSchemaModels 三方言 DDL 与实体清单双向对账。
func TestDDLMatchesSchemaModels(t *testing.T) {
	entities := manager.SchemaModels()
	for dialect, text := range ddl.All() {
		ddlTables := parseDDL(t, dialect, text)
		entTables := parseEntities(t, entities)

		for name := range ddlTables {
			if _, ok := entTables[name]; !ok {
				t.Errorf("[%s] DDL 有表 %s，但 SchemaModels 里没有对应实体", dialect, name)
			}
		}
		for name, ent := range entTables {
			ddlTable, ok := ddlTables[name]
			if !ok {
				t.Errorf("[%s] 实体表 %s 在 DDL 里缺失", dialect, name)
				continue
			}
			// 列：实体 → DDL，DDL → 实体（白名单除外）。
			for col := range ent.columns {
				if !ddlTable.columns[col] {
					t.Errorf("[%s] %s: 实体有列 %s，DDL 缺失", dialect, name, col)
				}
			}
			for col := range ddlTable.columns {
				if ent.columns[col] || ddlOnlyColumn(dialect, name, col) {
					continue
				}
				t.Errorf("[%s] %s: DDL 有列 %s，实体没有（且不在白名单）", dialect, name, col)
			}
			// 索引：双向。
			for idx := range ent.indexes {
				if !ddlTable.indexes[idx] {
					t.Errorf("[%s] %s: 实体有索引 %s，DDL 缺失", dialect, name, idx)
				}
			}
			for idx := range ddlTable.indexes {
				if ent.indexes[idx] || ddlOnlyIndex(dialect, name, idx) {
					continue
				}
				t.Errorf("[%s] %s: DDL 有索引 %s，实体没有（且不在白名单）", dialect, name, idx)
			}
		}
	}
}

// TestApplyIdempotent 三方言 DDL 连跑两遍都成功（第二遍只剩 already exists，不报错）。
func TestApplyIdempotent(t *testing.T) {
	for _, dialect := range []string{ddl.DialectSQLite, ddl.DialectPostgres, ddl.DialectMySQL} {
		t.Run(dialect, func(t *testing.T) {
			db := testdb.Open(t, dialect)
			testdb.Reset(t, db) // 真库可能残留旧版表结构，先整表重建
			testdb.Apply(t, db) // 第二遍：幂等（PG/SQLite 只 NOTICE，MySQL 只 IF NOT EXISTS 跳过）
			for _, m := range manager.SchemaModels() {
				if !db.Migrator().HasTable(m) {
					t.Fatalf("[%s] 表 %T 不存在", dialect, m)
				}
			}
		})
	}
}

// ---- DDL 解析：语句由 ddl.Statements 提供（已去注释），这里只做轻量名字扫描 ----

// ddlTable 是从 DDL 里扫出来的表：列名集合 + 索引名集合。
type ddlTable struct {
	columns map[string]bool
	indexes map[string]bool
}

// entityTable 是从 gorm 实体解析出来的表：列名集合 + 索引名集合。
type entityTable struct {
	columns map[string]bool
	indexes map[string]bool
}

func parseDDL(t *testing.T, dialect, text string) map[string]*ddlTable {
	t.Helper()
	stmts, err := ddl.Statements(dialect)
	if err != nil {
		t.Fatalf("[%s] statements: %v", dialect, err)
	}
	out := map[string]*ddlTable{}
	for _, stmt := range stmts {
		upper := strings.ToUpper(stmt)
		switch {
		case strings.HasPrefix(upper, "CREATE TABLE"):
			name, body := tableNameAndBody(t, dialect, stmt)
			tbl := out[name]
			if tbl == nil {
				tbl = &ddlTable{columns: map[string]bool{}, indexes: map[string]bool{}}
				out[name] = tbl
			}
			for _, item := range splitTopLevel(body) {
				addTableItem(tbl, item)
			}
		case strings.HasPrefix(upper, "CREATE INDEX"), strings.HasPrefix(upper, "CREATE UNIQUE INDEX"):
			name, table := parseCreateIndex(t, dialect, stmt)
			tbl := out[table]
			if tbl == nil {
				tbl = &ddlTable{columns: map[string]bool{}, indexes: map[string]bool{}}
				out[table] = tbl
			}
			tbl.indexes[name] = true
		}
	}
	if len(out) != 3 {
		t.Fatalf("[%s] 解析出 %d 张表，期望 3：%v", dialect, len(out), keysOfTables(out))
	}
	return out
}

// tableNameAndBody 取 CREATE TABLE 的表名与括号内定义体（深度扫描，兼容 MySQL 生成列里的括号）。
func tableNameAndBody(t *testing.T, dialect, stmt string) (string, string) {
	t.Helper()
	const prefix = "CREATE TABLE IF NOT EXISTS"
	rest := strings.TrimSpace(stmt[len(prefix):])
	sp := strings.IndexAny(rest, " \t\n(")
	if sp < 0 {
		t.Fatalf("[%s] 无法解析 CREATE TABLE: %.80s", dialect, stmt)
	}
	name := strings.TrimSpace(rest[:sp])
	rest = rest[sp:]
	open := strings.Index(rest, "(")
	if open < 0 {
		t.Fatalf("[%s] CREATE TABLE %s 缺左括号", dialect, name)
	}
	depth := 0
	for i := open; i < len(rest); i++ {
		switch rest[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return name, rest[open+1 : i]
			}
		}
	}
	t.Fatalf("[%s] CREATE TABLE %s 括号不闭合", dialect, name)
	return "", ""
}

// addTableItem 解析 CREATE TABLE 体内的一项：列定义或内联 KEY。
func addTableItem(tbl *ddlTable, item string) {
	item = strings.TrimSpace(item)
	upper := strings.ToUpper(item)
	switch {
	case strings.HasPrefix(upper, "PRIMARY KEY"):
		return // 主键不是命名索引，实体侧不比对
	case strings.HasPrefix(upper, "UNIQUE KEY "):
		tbl.indexes[secondToken(item, "UNIQUE KEY")] = true
	case strings.HasPrefix(upper, "KEY "), strings.HasPrefix(upper, "INDEX "):
		tbl.indexes[secondToken(item, "KEY")] = true
	case strings.HasPrefix(upper, "CONSTRAINT "), strings.HasPrefix(upper, "FOREIGN KEY"):
		return
	default:
		// 列定义：第一个 token 是列名（MySQL 生成列跨行也只取首个 token）。
		fields := strings.Fields(item)
		if len(fields) == 0 {
			return
		}
		tbl.columns[fields[0]] = true
	}
}

// secondToken 取 "KEY idx_xxx (...)" 这类定义里的索引名（跳过可能出现的 INDEX 关键字）。
func secondToken(item, keyword string) string {
	rest := strings.TrimSpace(item[len(keyword):])
	if strings.HasPrefix(strings.ToUpper(rest), "INDEX ") {
		rest = strings.TrimSpace(rest[len("INDEX"):])
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return ""
	}
	return strings.TrimRight(fields[0], "(")
}

// parseCreateIndex 解析 CREATE [UNIQUE] INDEX [IF NOT EXISTS] name ON table (...)。
func parseCreateIndex(t *testing.T, dialect, stmt string) (name, table string) {
	t.Helper()
	rest := strings.TrimSpace(stmt)
	rest = strings.TrimPrefix(rest, "CREATE ")
	if strings.HasPrefix(strings.ToUpper(rest), "UNIQUE ") {
		rest = rest[len("UNIQUE "):]
	}
	rest = rest[len("INDEX "):]
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(strings.ToUpper(rest), "IF NOT EXISTS ") {
		rest = rest[len("IF NOT EXISTS "):]
	}
	fields := strings.Fields(rest)
	if len(fields) < 3 || !strings.EqualFold(fields[1], "ON") {
		t.Fatalf("[%s] 无法解析 CREATE INDEX: %.80s", dialect, stmt)
	}
	return fields[0], strings.TrimRight(fields[2], "(")
}

// splitTopLevel 按顶层逗号切分（跳过括号内逗号，兼容 MySQL 生成列里的 IF(..., ..., ...)）。
func splitTopLevel(body string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(body); i++ {
		switch body[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, body[start:i])
				start = i + 1
			}
		}
	}
	if s := strings.TrimSpace(body[start:]); s != "" {
		out = append(out, s)
	}
	return out
}

// ---- 实体解析：gorm schema.Parse 取列名与索引名 ----

var schemaCache sync.Map

func parseEntities(t *testing.T, models []any) map[string]*entityTable {
	t.Helper()
	out := map[string]*entityTable{}
	for _, m := range models {
		sch, err := schema.Parse(m, &schemaCache, schema.NamingStrategy{})
		if err != nil {
			t.Fatalf("parse schema %T: %v", m, err)
		}
		tbl := &entityTable{columns: map[string]bool{}, indexes: map[string]bool{}}
		for _, f := range sch.Fields {
			if f.DBName == "" {
				continue
			}
			tbl.columns[f.DBName] = true
		}
		for _, idx := range sch.ParseIndexes() {
			tbl.indexes[idx.Name] = true
		}
		out[sch.Table] = tbl
	}
	return out
}

func keysOfTables(m map[string]*ddlTable) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
