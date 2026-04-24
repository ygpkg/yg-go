package model

import (
	"encoding/json"
	"gorm.io/gorm"
)

// TableNameCorePrompt 变式题目集 prompt 主表名
const TableNameCorePrompt = "core_prompt"

// PromptStatus prompt 启用/禁用状态枚举
type PromptStatus int

// PromptStatusEnabled 启用；PromptStatusDisabled 禁用
const (
	PromptStatusEnabled  PromptStatus = 1
	PromptStatusDisabled PromptStatus = 0
)

// EnumType prompt 变量键值类型，区分 string/int/enum/bool/float/list
type EnumType string

// VarKeyTypeString 字符串类型；VarKeyTypeInt 整数类型；VarKeyTypeEnum 枚举类型；
// VarKeyTypeBool 布尔类型；VarKeyTypeFloat 浮点类型；VarKeyTypeList 列表类型
const (
	VarKeyTypeString EnumType = "string"
	VarKeyTypeInt    EnumType = "int"
	VarKeyTypeEnum   EnumType = "enum"
	VarKeyTypeBool   EnumType = "bool"
	VarKeyTypeFloat  EnumType = "float"
	VarKeyTypeList   EnumType = "list"
)

// VarKey declares a prompt template variable, including its name, type, validation rules, and default value.
type VarKey struct {
	Name       string   `json:"name"`
	Type       EnumType `json:"type"`
	Desc       string   `json:"desc"`
	Required   bool     `json:"required"`
	EnumValues []string `json:"enum_values"`
	Default    any      `json:"default"`
	MaxLength  int      `json:"max_length"`
	Min        *int     `json:"min"`
	Max        *int     `json:"max"`
	MaxItems   int      `json:"max_items"`
}

// CorePrompt prompt 模板主表实体，归属公司并按应用+业务分组划分，code 用于业务硬编码查找
type CorePrompt struct {
	gorm.Model
	CompanyID       uint         `gorm:"column:company_id;type:bigint unsigned;not null;comment:公司ID;index:idx_cp_company" json:"company_id"`
	Uin             uint         `gorm:"column:uin;type:bigint unsigned;not null;default:0;comment:创建人UIN" json:"uin"`
	App             string       `gorm:"column:app;type:varchar(64);not null;comment:所属应用(如dotteacher/dotworker)" json:"app"`
	Group           string       `gorm:"column:group;type:varchar(64);not null;comment:业务分组(如question_variant)" json:"group"`
	Name            string       `gorm:"column:name;type:varchar(128);not null;comment:模板名称" json:"name"`
	Code            string       `gorm:"column:code;type:varchar(64);not null;comment:模板编码(业务硬编码查找);index:idx_cp_code,unique" json:"code"`
	LatestVersionID uint         `gorm:"column:latest_version_id;type:bigint unsigned;not null;default:0;comment:当前生效的最新版本ID" json:"latest_version_id"`
	Status          PromptStatus `gorm:"column:status;type:tinyint;not null;default:1;comment:1启用 0禁用" json:"status"`
	CreatedUin      uint         `gorm:"column:created_uin;type:bigint unsigned;not null;default:0;comment:创建人UIN" json:"created_uin"`
	UpdatedUin      uint         `gorm:"column:updated_uin;type:bigint unsigned;not null;default:0;comment:更新人UIN" json:"updated_uin"`
}

// TableName 返回 CorePrompt 表名
func (CorePrompt) TableName() string {
	return TableNameCorePrompt
}

// CorePromptList is a list alias of CorePrompt that provides collection methods such as ToMap.
type CorePromptList []CorePrompt

// ToMap 按 ID 为键将列表转为 map
func (l CorePromptList) ToMap() map[uint]CorePrompt {
	m := make(map[uint]CorePrompt, len(l))
	for _, v := range l {
		m[v.ID] = v
	}
	return m
}

// TableNameCorePromptVersion prompt 版本表名
const TableNameCorePromptVersion = "core_prompt_version"

// CorePromptVersion prompt 版本实体，记录某次模板内容与变量声明的快照
type CorePromptVersion struct {
	gorm.Model
	CompanyID    uint            `gorm:"column:company_id;type:bigint unsigned;not null;comment:公司ID" json:"company_id"`
	Uin          uint            `gorm:"column:uin;type:bigint unsigned;not null;default:0;comment:创建人UIN" json:"uin"`
	PromptID     uint            `gorm:"column:prompt_id;type:bigint unsigned;not null;comment:关联主表ID;index:idx_cpv_tid" json:"prompt_id"`
	Content      string          `gorm:"column:content;type:text;not null;comment:模板内容(Go template语法,包含占位符{{.VarName}})" json:"content"`
	VariableKeys json.RawMessage `gorm:"column:variable_keys;type:json;comment:本版本声明的变量清单(含名称、类型、校验规则)" json:"variable_keys"`
	CreatedUin   uint            `gorm:"column:created_uin;type:bigint unsigned;not null;default:0;comment:创建人UIN" json:"created_uin"`
	UpdatedUin   uint            `gorm:"column:updated_uin;type:bigint unsigned;not null;default:0;comment:更新人UIN" json:"updated_uin"`
}

// TableName 返回 CorePromptVersion 表名
func (CorePromptVersion) TableName() string {
	return TableNameCorePromptVersion
}

// CorePromptVersionList is a list alias of CorePromptVersion that provides collection methods such as ToMap.
type CorePromptVersionList []CorePromptVersion

// ToMap 按 ID 为键将列表转为 map
func (l CorePromptVersionList) ToMap() map[uint]CorePromptVersion {
	m := make(map[uint]CorePromptVersion, len(l))
	for _, v := range l {
		m[v.ID] = v
	}
	return m
}
