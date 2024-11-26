package apiobj

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

const (
	// DefaultPageSize 返回列表的默认长度
	DefaultPageSize = 10
	// PageMaxCount 返回列表的最大长度
	PageMaxCount = 150
)

var (
	earliestTime = time.Date(2015, time.December, 0, 0, 0, 0, 0, time.Local).Unix()
)

// allowOrderFielder 允许排序字段
type allowOrderFielder interface {
	AllowOrderFields() []string
}

// allowFilterFielder 允许过滤字段
type allowFilterFielder interface {
	AllowFilterFields() []string
}

// PageQuery .
type PageQuery struct {
	Offset    int       `json:",omitempty"`
	Limit     int       `json:",omitempty"`
	ListAll   bool      `json:",omitempty"`
	OrderBy   []string  `json:",omitempty"`
	Filters   []Filter  `json:",omitempty"`
	BeginTime time.Time `json:",omitempty"`
	EndTime   time.Time `json:",omitempty"`

	IsBackend  bool `json:"-"`
	CompanyID  uint `json:"-"` // CompanyID 大客户企业id
	EmployeeID uint `json:"-"` // DcEmployeeID 大客户员工id
	CustomerID uint `json:"-"` // HsID 健康师的客户id
	Uin        uint `json:"-"` // Uin 用户id
}

// Filter 过滤条件
type Filter struct {
	Field      string
	Value      []string
	ExactMatch bool
}

// Fill 填充值
func (p *PageQuery) Fill(req *http.Request) {
	if p.Offset <= 0 {
		p.Offset = 0
	}
	if p.Limit <= 0 || p.Limit > PageMaxCount {
		p.Limit = DefaultPageSize
	}
	if p.ListAll {
		p.Limit = PageMaxCount
	}
}

// IsValite 检查是否合法
func (p PageQuery) IsValite(allower interface{}) error {
	if p.Offset < 0 {
		return fmt.Errorf("offset is invalid, %v", p.Offset)
	}
	if p.Limit <= 0 || p.Limit > PageMaxCount {
		return fmt.Errorf("limit is invalid, %v", p.Limit)
	}

	var allowOrderFields, allowFilterFields []string
	if allower != nil {
		if orderAllower, ok := allower.(allowOrderFielder); ok {
			allowOrderFields = orderAllower.AllowOrderFields()
		}
		if filterAllower, ok := allower.(allowFilterFielder); ok {
			allowFilterFields = filterAllower.AllowFilterFields()
		}
	}

	if allowOrderFields != nil {
		for _, ob := range p.OrderBy {
			ob = strings.TrimSpace(ob)
			ob = strings.ToLower(ob)
			ob = strings.TrimSuffix(ob, " desc")
			ob = strings.TrimSuffix(ob, " asc")
			if !containsString(allowOrderFields, ob) {
				return fmt.Errorf("不支持的排序字段: %s" + ob)
			}
		}
	}
	if allowFilterFields != nil {
		for _, f := range p.Filters {
			if !containsString(allowFilterFields, f.Field) {
				return fmt.Errorf("不支持的过滤字段: %s" + f.Field)
			}
			if len(f.Value) == 0 {
				return fmt.Errorf("过滤字段值不能为空: %v" + f.Field)
			}
			if strings.HasSuffix(f.Field, "_at") {
				if len(f.Value) != 2 {
					return fmt.Errorf("时间过滤字段值必须为两个: %v" + f.Field)
				}
			}
		}
	}
	return nil
}

// QueryResponse .
type QueryResponse struct {
	Total  int64 `json:"total"`
	Offset int   `json:"offset"`
	Limit  int   `json:"limit"`
}

// containsString 判断字符串是否在字符串数组中
func containsString(arr []string, str string) bool {
	for _, v := range arr {
		if v == str {
			return true
		}
	}
	return false
}
