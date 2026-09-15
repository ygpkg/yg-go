package gormdao

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuildOrClause_SingleConditionPerGroup(t *testing.T) {
	groups := []OrCond{
		{CondGroups: []OrCondGroup{{Query: "type = ?", Args: []any{1}}}},
		{CondGroups: []OrCondGroup{{Query: "code = ?", Args: []any{"zzds"}}}},
	}
	query, args := buildOrClause("company", groups)
	assert.Equal(t, "(company.type = ? OR company.code = ?)", query)
	assert.Equal(t, []any{1, "zzds"}, args)
}

func TestBuildOrClause_MultiConditionInGroup(t *testing.T) {
	groups := []OrCond{
		{CondGroups: []OrCondGroup{{Query: "type = ?", Args: []any{1}}}},
		{CondGroups: []OrCondGroup{{Query: "name = ?", Args: []any{"2"}}, {Query: "seq = ?", Args: []any{2}}}},
		{CondGroups: []OrCondGroup{{Query: "code = ?", Args: []any{"zzds"}}}},
	}
	query, args := buildOrClause("company", groups)
	assert.Equal(t, "(company.type = ? OR (company.name = ? AND company.seq = ?) OR company.code = ?)", query)
	assert.Equal(t, []any{1, "2", 2, "zzds"}, args)
}

func TestBuildOrClause_AllMultiConditionGroups(t *testing.T) {
	groups := []OrCond{
		{CondGroups: []OrCondGroup{{Query: "name = ?", Args: []any{"a"}}, {Query: "seq = ?", Args: []any{1}}}},
		{CondGroups: []OrCondGroup{{Query: "name = ?", Args: []any{"b"}}, {Query: "seq = ?", Args: []any{2}}}},
	}
	query, args := buildOrClause("company", groups)
	assert.Equal(t, "((company.name = ? AND company.seq = ?) OR (company.name = ? AND company.seq = ?))", query)
	assert.Equal(t, []any{"a", 1, "b", 2}, args)
}

func TestBuildOrClause_SingleGroupSingleCondition(t *testing.T) {
	groups := []OrCond{
		{CondGroups: []OrCondGroup{{Query: "type = ?", Args: []any{1}}}},
	}
	query, args := buildOrClause("company", groups)
	assert.Equal(t, "(company.type = ?)", query)
	assert.Equal(t, []any{1}, args)
}

func TestBuildOrClause_EmptyConditionsInGroup(t *testing.T) {
	groups := []OrCond{
		{CondGroups: []OrCondGroup{{Query: "type = ?", Args: []any{1}}}},
		{CondGroups: []OrCondGroup{}},
	}
	query, args := buildOrClause("company", groups)
	assert.Equal(t, "(company.type = ?)", query)
	assert.Equal(t, []any{1}, args)
}

func TestBuildOrClause_AllEmptyGroups(t *testing.T) {
	groups := []OrCond{
		{CondGroups: []OrCondGroup{}},
		{CondGroups: nil},
	}
	query, args := buildOrClause("company", groups)
	assert.Equal(t, "", query)
	assert.Nil(t, args)

	// 空条件不应生成非法的 "()" SQL
	groups = []OrCond{{}}
	query, args = buildOrClause("company", groups)
	assert.Equal(t, "", query)
	assert.Nil(t, args)
}

func TestBaseCond_IncludeDeleted(t *testing.T) {
	assert.False(t, (&BaseCond{}).IncludeDeleted())
	assert.True(t, (&BaseCond{IsDelete: true}).IncludeDeleted())

	// 自定义 Cond 内嵌 BaseCond 时自动继承
	type customCond struct {
		BaseCond
	}
	assert.True(t, (&customCond{BaseCond: BaseCond{IsDelete: true}}).IncludeDeleted())
}

func TestBaseCond_GetOffsetInfoNilSafe(t *testing.T) {
	// 自定义 Cond 内嵌 *BaseCond 指针且未显式初始化时，BaseCond 为 nil。
	// 提升方法 GetOffsetInfo 在 nil 嵌入指针上调用不得 panic，应视为不分页。
	type customCond struct {
		*BaseCond
	}
	cond := &customCond{}
	offset, limit := cond.GetOffsetInfo()
	assert.Zero(t, offset)
	assert.Zero(t, limit)

	// 显式初始化时正常返回分页参数
	cond = &customCond{BaseCond: &BaseCond{Offset: 20, Limit: 10}}
	offset, limit = cond.GetOffsetInfo()
	assert.Equal(t, 20, offset)
	assert.Equal(t, 10, limit)
}
