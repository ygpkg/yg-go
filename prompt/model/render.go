package model

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"text/template"
	"text/template/parse"

	"github.com/ygpkg/yg-go/logs"
)

// DefaultFuncMap prompt 模板渲染使用的自定义函数映射，包含 Upper/Lower/Trunct/JoinComma
var DefaultFuncMap = template.FuncMap{
	"Upper":     strings.ToUpper,
	"Lower":     strings.ToLower,
	"Trunct":    truncFunc,
	"JoinComma": joinCommaFunc,
}

// truncFunc 截断字符串至指定长度，超出部分加省略号
func truncFunc(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// joinCommaFunc 将字符串切片用逗号连接
func joinCommaFunc(items []string) string {
	return strings.Join(items, ", ")
}

// ExtractVariableNames 从 Go template AST 中提取所有变量引用名（{{.VarName}}），返回去重后的集合
func ExtractVariableNames(content string) ([]string, error) {
	t, err := template.New("prompt_extract").Parse(content)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrTemplateParseFailed, err)
	}

	seen := make(map[string]bool)
	var names []string
	walkAST(t.Tree.Root, seen)
	for name := range seen {
		names = append(names, name)
	}
	return names, nil
}

// walkAST 递归遍历模板 AST 的 Node 列表，提取 NodeField 和 NodeVariable 中的变量名
func walkAST(node parse.Node, seen map[string]bool) {
	switch n := node.(type) {
	case *parse.ListNode:
		for _, kid := range n.Nodes {
			walkAST(kid, seen)
		}
	case *parse.ActionNode:
		walkAST(n.Pipe, seen)
	case *parse.PipeNode:
		for _, cmd := range n.Cmds {
			walkAST(cmd, seen)
		}
	case *parse.CommandNode:
		for _, arg := range n.Args {
			walkAST(arg, seen)
		}
	case *parse.FieldNode:
		for _, ident := range n.Ident {
			seen[ident] = true
		}
	case *parse.VariableNode:
		for i, ident := range n.Ident {
			if i == 0 && ident == "$" {
				continue
			}
			seen[ident] = true
		}
	case *parse.IfNode:
		walkAST(n.Pipe, seen)
		walkAST(n.List, seen)
		if n.ElseList != nil {
			walkAST(n.ElseList, seen)
		}
	case *parse.RangeNode:
		walkAST(n.Pipe, seen)
		walkAST(n.List, seen)
		if n.ElseList != nil {
			walkAST(n.ElseList, seen)
		}
	case *parse.WithNode:
		walkAST(n.Pipe, seen)
		walkAST(n.List, seen)
		if n.ElseList != nil {
			walkAST(n.ElseList, seen)
		}
	}
}

// ParseVariableKeys 从 content AST 提取变量名，与声明的 variable_keys 交叉校验
// 允许 variable_keys 声明多于 AST 引用的变量，不允许 AST 引用但未声明
func ParseVariableKeys(ctx context.Context, content string, declaredKeys []VarKey) ([]VarKey, error) {
	if content == "" {
		return nil, ErrEmptyContent
	}

	astNames, err := ExtractVariableNames(content)
	if err != nil {
		logs.ErrorContextf(ctx, "[ParseVariableKeys] extract variable names failed, err: %v", err)
		return nil, err
	}

	declaredNameSet := make(map[string]bool, len(declaredKeys))
	for _, key := range declaredKeys {
		declaredNameSet[key.Name] = true
	}

	var undeclared []string
	for _, name := range astNames {
		if !declaredNameSet[name] {
			undeclared = append(undeclared, name)
		}
	}

	if len(undeclared) > 0 {
		logs.ErrorContextf(ctx, "[ParseVariableKeys] undeclared variables: %v", undeclared)
		return nil, fmt.Errorf("%w: %v", ErrUndeclaredVariable, undeclared)
	}

	for _, key := range declaredKeys {
		if !isValidEnumType(key.Type) {
			logs.ErrorContextf(ctx, "[ParseVariableKeys] invalid variable key type: %s, name: %s", key.Type, key.Name)
			return nil, fmt.Errorf("%w: name=%s, type=%s", ErrInvalidVarKeyType, key.Name, key.Type)
		}
	}

	return declaredKeys, nil
}

// isValidEnumType 校验 EnumType 是否为合法的变量类型枚举值
func isValidEnumType(t EnumType) bool {
	switch t {
	case VarKeyTypeString, VarKeyTypeInt, VarKeyTypeEnum,
		VarKeyTypeBool, VarKeyTypeFloat, VarKeyTypeList:
		return true
	default:
		return false
	}
}

// ValidatePromptValue 按 variable_keys 校验传入的 prompt_value
// 校验维度：required、类型匹配、enum 取值范围、int/float 范围、string 长度、list 项数
// 非 required 且缺失的变量，使用 VarKey.Default 填充
func ValidatePromptValue(ctx context.Context, keys []VarKey, values map[string]any) (map[string]any, error) {
	result := make(map[string]any, len(keys))

	for _, key := range keys {
		val, exists := values[key.Name]

		if !exists || val == nil {
			if key.Required {
				logs.ErrorContextf(ctx, "[ValidatePromptValue] missing required variable: %s", key.Name)
				return nil, fmt.Errorf("%w: %s", ErrMissingRequiredVar, key.Name)
			}
			if key.Default != nil {
				result[key.Name] = key.Default
			}
			continue
		}

		switch key.Type {
		case VarKeyTypeString:
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("%w: %s expected string", ErrInvalidValueType, key.Name)
			}
			if key.MaxLength > 0 && len(s) > key.MaxLength {
				return nil, fmt.Errorf("%w: %s len=%d, max_length=%d", ErrStringLengthExceeded, key.Name, len(s), key.MaxLength)
			}
			result[key.Name] = s

		case VarKeyTypeInt:
			var intVal int
			switch v := val.(type) {
			case int:
				intVal = v
			case int64:
				intVal = int(v)
			case float64:
				intVal = int(v)
			default:
				return nil, fmt.Errorf("%w: %s expected int", ErrInvalidValueType, key.Name)
			}
			if key.Min != nil && intVal < *key.Min {
				return nil, fmt.Errorf("%w: %s val=%d, min=%d", ErrIntRangeExceeded, key.Name, intVal, *key.Min)
			}
			if key.Max != nil && intVal > *key.Max {
				return nil, fmt.Errorf("%w: %s val=%d, max=%d", ErrIntRangeExceeded, key.Name, intVal, *key.Max)
			}
			result[key.Name] = intVal

		case VarKeyTypeEnum:
			s, ok := val.(string)
			if !ok {
				return nil, fmt.Errorf("%w: %s expected string for enum", ErrInvalidValueType, key.Name)
			}
			found := false
			for _, ev := range key.EnumValues {
				if s == ev {
					found = true
					break
				}
			}
			if !found {
				return nil, fmt.Errorf("%w: %s val=%s, enum_values=%v", ErrEnumValueMismatch, key.Name, s, key.EnumValues)
			}
			result[key.Name] = s

		case VarKeyTypeBool:
			b, ok := val.(bool)
			if !ok {
				return nil, fmt.Errorf("%w: %s expected bool", ErrInvalidValueType, key.Name)
			}
			result[key.Name] = b

		case VarKeyTypeFloat:
			floatVal, ok := val.(float64)
			if !ok {
				return nil, fmt.Errorf("%w: %s expected float", ErrInvalidValueType, key.Name)
			}
			if key.Min != nil && floatVal < float64(*key.Min) {
				return nil, fmt.Errorf("%w: %s val=%v, min=%d", ErrFloatRangeExceeded, key.Name, floatVal, *key.Min)
			}
			if key.Max != nil && floatVal > float64(*key.Max) {
				return nil, fmt.Errorf("%w: %s val=%v, max=%d", ErrFloatRangeExceeded, key.Name, floatVal, *key.Max)
			}
			result[key.Name] = floatVal

		case VarKeyTypeList:
			items, ok := val.([]any)
			if !ok {
				return nil, fmt.Errorf("%w: %s expected list", ErrInvalidValueType, key.Name)
			}
			if key.MaxItems > 0 && len(items) > key.MaxItems {
				return nil, fmt.Errorf("%w: %s len=%d, max_items=%d", ErrListTooManyItems, key.Name, len(items), key.MaxItems)
			}
			result[key.Name] = items

		default:
			return nil, fmt.Errorf("%w: %s type=%s", ErrInvalidVarKeyType, key.Name, key.Type)
		}
	}

	return result, nil
}

// ValidateAndRender 校验参数 + 渲染模板，返回最终 Prompt 字符串
// 流程：ParseVariableKeys → ValidatePromptValue → text/template Execute
func ValidateAndRender(ctx context.Context, content string, variableKeys []VarKey, promptValue map[string]any) (string, error) {
	if content == "" {
		return "", ErrEmptyContent
	}

	_, err := ParseVariableKeys(ctx, content, variableKeys)
	if err != nil {
		logs.ErrorContextf(ctx, "[ValidateAndRender] ParseVariableKeys failed, err: %v", err)
		return "", err
	}

	validatedValues, err := ValidatePromptValue(ctx, variableKeys, promptValue)
	if err != nil {
		logs.ErrorContextf(ctx, "[ValidateAndRender] ValidatePromptValue failed, err: %v", err)
		return "", err
	}

	t, err := template.New("prompt_render").Funcs(DefaultFuncMap).Parse(content)
	if err != nil {
		logs.ErrorContextf(ctx, "[ValidateAndRender] template parse failed, err: %v", err)
		return "", fmt.Errorf("%w: %v", ErrTemplateParseFailed, err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, validatedValues); err != nil {
		logs.ErrorContextf(ctx, "[ValidateAndRender] template render failed, err: %v", err)
		return "", fmt.Errorf("%w: %v", ErrTemplateRenderFailed, err)
	}

	return buf.String(), nil
}
