package gormdao

import (
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// OrCondGroup 单个 AND 条件组，Query 中可含占位符，Args 为对应参数。
type OrCondGroup struct {
	Query string
	Args  []any
}

// OrCond 一组 OR 条件：组内各 CondGroup 以 AND 连接，组间以 OR 连接。
type OrCond struct {
	CondGroups []OrCondGroup
}

// Cond 查询条件接口。
type Cond interface {
	BuildCondition(db *gorm.DB, tableName string)
	GetOffsetInfo() (int, int)
}

// BaseCond 通用查询条件。
// ID/IDs 为 any，支持 uint / int64 / string 等任意主键类型；
// 主键类型的编译期约束由 Dao 的 IDType 承担，Cond 作为纯数据容器保持非泛型。
type BaseCond struct {
	ID  any
	IDs []any
	// IsDelete 为 true 时查询包含已删除的记录（软删除场景下等价于 Unscoped）。
	IsDelete       bool
	Offset         int
	Limit          int
	CreatedAtStart int64
	CreatedAtEnd   int64
	OrderField     string
	OrConditions   []OrCond
}

func (c *BaseCond) BuildCondition(db *gorm.DB, tableName string) {
	BuildBaseCondition(db, tableName, c)
}

// GetOffsetInfo 返回分页参数 (offset, limit)。
// nil 安全：内嵌 *BaseCond 的 Cond 常以零值构造（未显式初始化 BaseCond），
// 此时 c 为 nil，直接访问 c.Offset 会 panic（promoted method on nil embedded pointer）。
func (c *BaseCond) GetOffsetInfo() (int, int) {
	if c == nil {
		return 0, 0
	}
	return c.Offset, c.Limit
}

// IncludeDeleted 返回是否查询包含已删除的记录，供 Dao 层软删除过滤使用。
// 自定义 Cond 若内嵌 BaseCond 则自动实现；也可自行实现该方法。
func (c *BaseCond) IncludeDeleted() bool {
	// nil 安全：内嵌 *BaseCond 的 Cond 常以零值构造，此时 c 为 nil。
	return c != nil && c.IsDelete
}

func BuildBaseCondition(db *gorm.DB, tableName string, cond *BaseCond) {
	if !isZeroAny(cond.ID) {
		query := fmt.Sprintf("%s.id = ?", tableName)
		db.Where(query, cond.ID)
	}
	if len(cond.IDs) > 0 {
		query := fmt.Sprintf("%s.id IN (?)", tableName)
		db.Where(query, cond.IDs)
	}
	if cond.CreatedAtStart > 0 {
		query := fmt.Sprintf("%s.created_at >= ?", tableName)
		db.Where(query, time.Unix(cond.CreatedAtStart, 0))
	}
	if cond.CreatedAtEnd > 0 {
		query := fmt.Sprintf("%s.created_at <= ?", tableName)
		db.Where(query, time.Unix(cond.CreatedAtEnd, 0))
	}
	if cond.IsDelete {
		db.Unscoped()
	}
	if cond.OrderField != "" {
		db.Order(cond.OrderField)
	}
	if len(cond.OrConditions) > 0 {
		query, args := buildOrClause(tableName, cond.OrConditions)
		if query != "" {
			db.Where(query, args...)
		}
	}
}

func buildOrClause(tableName string, orConditions []OrCond) (string, []any) {
	var args []any
	parts := make([]string, 0, len(orConditions))
	for _, orCond := range orConditions {
		if len(orCond.CondGroups) == 0 {
			continue
		}
		subParts := make([]string, 0, len(orCond.CondGroups))
		for _, orCondGroup := range orCond.CondGroups {
			subParts = append(subParts, fmt.Sprintf("%s.%s", tableName, orCondGroup.Query))
			args = append(args, orCondGroup.Args...)
		}
		if len(subParts) == 1 {
			parts = append(parts, subParts[0])
		} else {
			parts = append(parts, "("+strings.Join(subParts, " AND ")+")")
		}
	}
	// 所有分组均为空时返回空串，由调用方跳过 db.Where，避免生成非法的 "()" 条件
	if len(parts) == 0 {
		return "", nil
	}
	return "(" + strings.Join(parts, " OR ") + ")", args
}
