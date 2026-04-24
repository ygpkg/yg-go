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

// DefaultFuncMap provides custom function mappings for prompt template rendering, including Upper/Lower/Trunct/JoinComma.
var DefaultFuncMap = template.FuncMap{
	"Upper":     strings.ToUpper,
	"Lower":     strings.ToLower,
	"Trunct":    truncFunc,
	"JoinComma": joinCommaFunc,
}

// truncFunc truncates a string to the specified length, appending ellipsis for the excess part.
func truncFunc(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen < 3 {
		return s[:maxLen]
	}
	return s[:maxLen-3] + "..."
}

// joinCommaFunc joins a string slice with commas.
func joinCommaFunc(items []string) string {
	return strings.Join(items, ", ")
}

// ExtractVariableNames extracts all variable reference names ({{.VarName}}) from the Go template AST and returns a deduplicated set.
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

// walkAST recursively traverses the template AST node list, extracting variable names from NodeField and NodeVariable.
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

// ParseVariableKeys extracts variable names from the content AST and cross-validates them with declared variable_keys.
// It allows variable_keys to declare more variables than the AST references, but forbids AST references that are undeclared.
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

// isValidEnumType validates whether an EnumType is a valid variable type enum value.
func isValidEnumType(t EnumType) bool {
	switch t {
	case VarKeyTypeString, VarKeyTypeInt, VarKeyTypeEnum,
		VarKeyTypeBool, VarKeyTypeFloat, VarKeyTypeList:
		return true
	default:
		return false
	}
}

// ValidatePromptValue validates the incoming prompt_value against variable_keys.
// Validation dimensions: required, type matching, enum value range, int/float range, string length, list item count.
// For non-required missing variables, VarKey.Default is used to fill.
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

// ValidateAndRender validates prompt variables and renders the template content into the final prompt string.
// Validation flow: ParseVariableKeys → ValidatePromptValue → text/template Execute
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
