package db

import (
	"fmt"
	"strings"
	"time"
)

// Dialect 标识底层数据库方言。认领、续租、回收、清理的 SQL 片段（时间算术、
// 锁定子句、冲突忽略、空键归一）都随方言变化，集中在这里生成，测试用快照断言。
type Dialect int

const (
	// DialectUnknown 未识别（构造期会退回 SQLite 语义，但应尽量避免）。
	DialectUnknown Dialect = iota
	// DialectMySQL MySQL 8.0.1+。
	DialectMySQL
	// DialectPostgres PostgreSQL 12+。
	DialectPostgres
	// DialectSQLite SQLite（测试用，纯 Go 驱动）。
	DialectSQLite
)

// String 返回方言短名，便于日志与测试断言。
func (d Dialect) String() string {
	switch d {
	case DialectMySQL:
		return "mysql"
	case DialectPostgres:
		return "postgres"
	case DialectSQLite:
		return "sqlite"
	default:
		return "unknown"
	}
}

// DetectDialect 根据 gorm 方言名识别 Dialect。
//
// gorm 的 dialector Name() 返回值：mysql 驱动为 "mysql"，postgres 驱动为
// "postgres"，本项目 sqlite 驱动为 "sqlite"。无法识别时返回 DialectUnknown。
func DetectDialect(name string) Dialect {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "mysql":
		return DialectMySQL
	case "postgres", "postgresql":
		return DialectPostgres
	case "sqlite", "sqlite3":
		return DialectSQLite
	default:
		return DialectUnknown
	}
}

// nowExpr 返回「当前 DB 时间（毫秒精度）」的 SQL 表达式。
//
// 时间一律走 DB 时钟：多实例时钟偏差不应影响调度与接管（见 internals「时间来源」）。
//   - MySQL：NOW(3)
//   - PostgreSQL：now()（now() 已是微秒精度）
//   - SQLite：CURRENT_TIMESTAMP 精度只到秒；须用 strftime 取毫秒并还原成
//     'YYYY-MM-DD HH:MM:SS.SSS' 的文本，供与数据库内时间列文本比较。
func (d Dialect) nowExpr() string {
	switch d {
	case DialectMySQL:
		return "NOW(3)"
	case DialectPostgres:
		return "now()"
	case DialectSQLite:
		// 存进来的时间列是 Go time.Time 的默认文本（如 "2026-11-15 16:09:19.05 +0800 CST"），
		// SQLite 的 Text 比较无法与 strftime 生成的 UTC 串直接对齐。这里统一返回
		// julianday('now')（一个 UTC 浮点标量），配合 colLE 对列侧也用 julianday() 包裹。
		return "julianday('now')"
	default:
		return "CURRENT_TIMESTAMP"
	}
}

// nowValueExpr 返回可写入时间列、及用于 COALESCE 的「当前 DB 时间」值表达式。
//
// 与 nowExpr 的区别：nowExpr 对 SQLite 返回 julianday('now')（比较用标量），
// 而写入列需要 datetime 文本，故 SQLite 这里用 datetime('now')。
func (d Dialect) nowValueExpr() string {
	switch d {
	case DialectSQLite:
		return "datetime('now')"
	default:
		return d.nowExpr()
	}
}

// colLE 返回「列 col <= now」的 SQL 布尔表达式。
//
//   - MySQL/PG：col <= <nowExpr>（列即时间类型，原生可比）
//   - SQLite：julianday(col) <= julianday('now')——列是文本且时区后缀不规范，
//     必须两侧都 julian 化后比较（实测 datetime(col)/julianday(col) 都能解析 Go 的
//     time.Time 文本并归一到 UTC）。
func (d Dialect) colLE(col string) string {
	switch d {
	case DialectSQLite:
		return fmt.Sprintf("julianday(%s) <= julianday('now')", col)
	default:
		return fmt.Sprintf("%s <= %s", col, d.nowExpr())
	}
}

// colLTNowExpr 返回「列 col < now」的 SQL 布尔表达式（用于租约过期判断）。
func (d Dialect) colLT(col string) string {
	switch d {
	case DialectSQLite:
		return fmt.Sprintf("julianday(%s) < julianday('now')", col)
	default:
		return fmt.Sprintf("%s < %s", col, d.nowExpr())
	}
}

// colGT 返回「列 col > now」的布尔表达式（判活：lease_until > now）。
//
// SQLite 下必须两侧都 julianday 化：lease_until 是 RFC3339 文本、'T' > ' '，
// 直接文本比较会与时间无关地恒真。
func (d Dialect) colGT(col string) string {
	switch d {
	case DialectSQLite:
		return fmt.Sprintf("julianday(%s) > julianday('now')", col)
	default:
		return fmt.Sprintf("%s > %s", col, d.nowExpr())
	}
}

// colLEAfter 返回「列 col <= now - dur」的布尔表达式（清理：finished_at < now-Retention）。
// 参数由调用方按 afterArg 的负数形式传入。
func (d Dialect) colLTBefore(col string) string {
	switch d {
	case DialectSQLite:
		return fmt.Sprintf("julianday(%s) < julianday('now', ?)", col)
	default:
		return fmt.Sprintf("%s < %s", col, d.beforeExpr())
	}
}

// beforeExpr 返回「now - dur」的时间表达式（参数占 ?）。
func (d Dialect) beforeExpr() string {
	switch d {
	case DialectMySQL:
		return fmt.Sprintf("DATE_SUB(%s, INTERVAL ? MICROSECOND)", d.nowExpr())
	case DialectPostgres:
		return fmt.Sprintf("%s - ? * interval '1 microsecond'", d.nowExpr())
	case DialectSQLite:
		return "julianday('now', ?)"
	default:
		return "CURRENT_TIMESTAMP"
	}
}

// beforeArg 把时长换算成 beforeExpr 的参数位应传的值（表示「往前推 dur」）。
//
//   - MySQL：DATE_SUB 自行做减法，传正微秒；
//   - PostgreSQL：表达式用减号，传正微秒；
//   - SQLite：modifier 是加法的，"往前" 需传负数秒。
func (d Dialect) beforeArg(dur time.Duration) any {
	switch d {
	case DialectMySQL:
		return dur.Microseconds()
	case DialectPostgres:
		return float64(dur.Microseconds())
	case DialectSQLite:
		return fmt.Sprintf("%0.3f seconds", -dur.Seconds())
	default:
		return dur.Microseconds()
	}
}

// afterExpr 返回「now + d」的 SQL 表达式，用参数占位 ? 承载间隔值。
//
// 间隔一律以「微秒」为参数单位（SQLite 是 N 秒浮点，见下），避免各数据库
// INTERVAL 字面量拼接带来的注入与类型问题。参数由调用方按 dialect 传入：
//   - MySQL：传 uint64 微秒，DATE_ADD(NOW(3), INTERVAL ? MICROSECOND)
//   - PostgreSQL：传 float64 微秒，now() + ? * interval '1 microsecond'
//   - SQLite：传字符串 '+N.MMM seconds'，datetime('now', ?)
func (d Dialect) afterExpr() string {
	switch d {
	case DialectMySQL:
		return fmt.Sprintf("DATE_ADD(%s, INTERVAL ? MICROSECOND)", d.nowExpr())
	case DialectPostgres:
		return fmt.Sprintf("%s + ? * interval '1 microsecond'", d.nowExpr())
	case DialectSQLite:
		return "datetime('now', ?)"
	default:
		return "CURRENT_TIMESTAMP"
	}
}

// afterArg 把时长换算成 afterExpr 对应参数位应传的值。
//
// MySQL/PG 以微秒为单位；SQLite 用 seconds 浮点（+0.500 seconds 表示半秒）。
func (d Dialect) afterArg(dur time.Duration) any {
	switch d {
	case DialectMySQL:
		return dur.Microseconds()
	case DialectPostgres:
		return float64(dur.Microseconds())
	case DialectSQLite:
		// 保留 3 位毫秒精度，避免秒以下被截断。
		return fmt.Sprintf("+%0.3f seconds", dur.Seconds())
	default:
		return dur.Microseconds()
	}
}

// claimLockClause 返回认领 SELECT 的锁定子句。
//
// MySQL / PostgreSQL 用 FOR UPDATE SKIP LOCKED 做原子认领；SQLite 无该子句，
// 退化为不加锁（去掉该子句）。SQLite 不验证并发（测试环境单连接），
// 生产不会用 SQLite 承载认领，故这一退化是安全的。
func (d Dialect) claimLockClause() string {
	switch d {
	case DialectMySQL, DialectPostgres:
		return " FOR UPDATE SKIP LOCKED"
	default:
		return ""
	}
}

// rowLockClause 返回「按主键锁住这一行」的子句（不加 SKIP LOCKED）。
//
// 用于 Save 在读-判-写之间锁住任务行：先确认仍持有租约，再跑业务钩子，
// 避免「守卫判定与钩子执行之间被接管」。
func (d Dialect) rowLockClause() string {
	switch d {
	case DialectMySQL, DialectPostgres:
		return " FOR UPDATE"
	default:
		return ""
	}
}

// emptyKeyExpr 返回「空串归一为 NULL」的表达式，用于 uniq_key 写入。
//
// 直接写空串会让唯一索引把所有「不带幂等键」的任务视作同一个键，
// 第二条提交会被静默吞掉。占位 ? 承载 Go 侧 string。
func (d Dialect) emptyKeyExpr() string {
	return "NULLIF(?, '')"
}

// insertIgnoreClause 返回「冲突忽略」的 INSERT 子句。
//
//   - MySQL：INSERT IGNORE INTO
//   - PostgreSQL / SQLite：INSERT ... ON CONFLICT DO NOTHING（追加在末尾）
//
// 返回值 (prefix, suffix)：prefix 拼在 INSERT 之后、表名之前，suffix 拼在
// 语句末尾。MySQL 用 prefix，PG/SQLite 用 suffix。
func (d Dialect) insertIgnoreClause() (prefix, suffix string) {
	switch d {
	case DialectMySQL:
		return "IGNORE ", ""
	default:
		return "", " ON CONFLICT DO NOTHING"
	}
}

// isolationReadCommittedSQL 返回「为下一个事务设置 READ COMMITTED」的语句。
//
// 该语句必须在 START TRANSACTION 之前执行（事务内执行报 MySQL 1568 且只对下
// 一个事务生效），且要绑定同一条独占连接，避免污染连接池。
//
//   - MySQL：SET TRANSACTION ISOLATION LEVEL READ COMMITTED
//   - PostgreSQL：SET TRANSACTION ISOLATION LEVEL READ COMMITTED（在事务开始后、
//     首条语句前执行才生效；gorm 用 BeginTx 的 Isolation 选项更稳，见 store）
//   - SQLite：空（SQLite 无隔离级别设置，单写者语义）
func (d Dialect) isolationReadCommittedSQL() string {
	switch d {
	case DialectMySQL:
		return "SET TRANSACTION ISOLATION LEVEL READ COMMITTED"
	case DialectPostgres:
		return "SET TRANSACTION ISOLATION LEVEL READ COMMITTED"
	default:
		return ""
	}
}

// leastLeaseOrNowExpr 返回「LEAST(COALESCE(lease, now), now)」的 SQL 表达式与参数，
// 用于回收器 ① 的 finished_at（取失权时刻，见 internals「回收器」）。
//
// leaseArg 是 select 出的 lease_until 值（可能 nil）。SQLite 需把结果落成驱动可解析的
// 时间文本：先 julianday 化取 min，再用 strftime 还原为 'YYYY-MM-DD HH:MM:SS.SSS'。
func (s *store) leastLeaseOrNow(leaseArg any, nowVal string) (string, []any) {
	switch s.dialect {
	case DialectSQLite:
		expr := "strftime('%Y-%m-%d %H:%M:%f', min(COALESCE(julianday(?), julianday('now')), julianday('now')))"
		return expr, []any{leaseArg}
	default:
		expr := fmt.Sprintf("LEAST(COALESCE(?, %s), %s)", nowVal, nowVal)
		return expr, []any{leaseArg}
	}
}

// ---- 导出包装：供 manager（提交侧 insert 语句）复用同一套方言片段，避免两处各写一份 ----

// NowValueExpr 返回当前时间的 SQL 值表达式（导出包装，见 nowValueExpr）。
func (d Dialect) NowValueExpr() string { return d.nowValueExpr() }

// AfterExpr 返回「当前时间 + 参数」的 SQL 表达式（导出包装，见 afterExpr）。
func (d Dialect) AfterExpr() string { return d.afterExpr() }

// AfterArg 返回与 AfterExpr 搭配的参数（导出包装，见 afterArg）。
func (d Dialect) AfterArg(dur time.Duration) any { return d.afterArg(dur) }

// EmptyKeyExpr 返回把空串归一为 NULL 的表达式（导出包装，见 emptyKeyExpr）。
func (d Dialect) EmptyKeyExpr() string { return d.emptyKeyExpr() }

// InsertIgnoreClause 返回「冲突忽略」插入的前后缀（导出包装，见 insertIgnoreClause）。
func (d Dialect) InsertIgnoreClause() (prefix, suffix string) { return d.insertIgnoreClause() }

// ColGT 返回「列 col > now」的布尔表达式（导出包装，见 colGT）。
func (d Dialect) ColGT(col string) string { return d.colGT(col) }

// jsonExtract 返回「按 key 精确匹配 value」的 WHERE 片段与参数（按方言）。
// MySQL: JSON_UNQUOTE(JSON_EXTRACT(col,'$.k')) = ?；Postgres: col->>'k' = ?；
// SQLite: json_extract(col,'$.k') = ?。key 由调用方保证为合法标识符（本包仅用于业务 label 名）。
func (d Dialect) jsonExtract(col, key, val string) (string, []any) {
	// labels 列是 text：PG 必须显式转 jsonb（text 没有 ->> 运算符），
	// 且三方言都要先排除 NULL/空串（空串不是合法 JSON，MySQL/SQLite 的
	// JSON_EXTRACT / json_extract 会直接报错）。
	guard := fmt.Sprintf("%s IS NOT NULL AND %s <> ''", col, col)
	switch d {
	case DialectPostgres:
		return fmt.Sprintf("%s AND (%s::jsonb) ->> ? = ?", guard, col), []any{key, val}
	case DialectMySQL:
		return fmt.Sprintf("%s AND JSON_UNQUOTE(JSON_EXTRACT(%s, ?)) = ?", guard, col), []any{"$." + key, val}
	default: // sqlite
		return fmt.Sprintf("%s AND json_extract(%s, ?) = ?", guard, col), []any{"$." + key, val}
	}
}
