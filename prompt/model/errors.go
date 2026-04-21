package model

import "errors"

// ErrEmptyContent prompt content 为空
var ErrEmptyContent = errors.New("prompt content is empty")

// ErrEmptyVariableKeys variable_keys 为空
var ErrEmptyVariableKeys = errors.New("variable_keys is empty")

// ErrPromptNotFound prompt 记录不存在
var ErrPromptNotFound = errors.New("prompt not found")

// ErrVersionNotFound prompt version 记录不存在
var ErrVersionNotFound = errors.New("prompt version not found")

// ErrUndeclaredVariable 模板中引用了未在 variable_keys 中声明的变量
var ErrUndeclaredVariable = errors.New("variable referenced in template but not declared in variable_keys")

// ErrInvalidVarKeyType variable_keys 中存在不合法的 type 值
var ErrInvalidVarKeyType = errors.New("invalid variable key type")

// ErrMissingRequiredVar prompt_value 缺少 required 变量
var ErrMissingRequiredVar = errors.New("missing required variable in prompt_value")

// ErrEnumValueMismatch prompt_value 中的值不在 enum_values 列表中
var ErrEnumValueMismatch = errors.New("value not in enum_values list")

// ErrIntRangeExceeded int 类型值超出 min/max 范围
var ErrIntRangeExceeded = errors.New("int value out of min/max range")

// ErrFloatRangeExceeded float 类型值超出 min/max 范围
var ErrFloatRangeExceeded = errors.New("float value out of min/max range")

// ErrStringLengthExceeded string 类型值超过 max_length
var ErrStringLengthExceeded = errors.New("string value exceeds max_length")

// ErrListTooManyItems list 类型值超过 max_items
var ErrListTooManyItems = errors.New("list exceeds max_items")

// ErrInvalidValueType prompt_value 中变量值类型与声明 type 不匹配
var ErrInvalidValueType = errors.New("variable value type does not match declared type")

// ErrTemplateParseFailed Go template 解析失败
var ErrTemplateParseFailed = errors.New("failed to parse template content")

// ErrTemplateRenderFailed Go template 渲染失败
var ErrTemplateRenderFailed = errors.New("failed to render template")
