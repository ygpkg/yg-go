package model

import "errors"

// ErrEmptyContent indicates that the prompt content is empty.
var ErrEmptyContent = errors.New("prompt content is empty")

// ErrEmptyVariableKeys indicates that variable_keys is empty.
var ErrEmptyVariableKeys = errors.New("variable_keys is empty")

// ErrPromptNotFound indicates that the prompt record does not exist.
var ErrPromptNotFound = errors.New("prompt not found")

// ErrVersionNotFound indicates that the prompt version record does not exist.
var ErrVersionNotFound = errors.New("prompt version not found")

// ErrUndeclaredVariable indicates a variable referenced in template but not declared in variable_keys.
var ErrUndeclaredVariable = errors.New("variable referenced in template but not declared in variable_keys")

// ErrInvalidVarKeyType indicates an invalid type value in variable_keys.
var ErrInvalidVarKeyType = errors.New("invalid variable key type")

// ErrMissingRequiredVar indicates a required variable is missing in prompt_value.
var ErrMissingRequiredVar = errors.New("missing required variable in prompt_value")

// ErrEnumValueMismatch indicates that a value is not in the enum_values list.
var ErrEnumValueMismatch = errors.New("value not in enum_values list")

// ErrIntRangeExceeded indicates that an int value exceeds the min/max range.
var ErrIntRangeExceeded = errors.New("int value out of min/max range")

// ErrFloatRangeExceeded indicates that a float value exceeds the min/max range.
var ErrFloatRangeExceeded = errors.New("float value out of min/max range")

// ErrStringLengthExceeded indicates that a string value exceeds max_length.
var ErrStringLengthExceeded = errors.New("string value exceeds max_length")

// ErrListTooManyItems indicates that a list exceeds max_items.
var ErrListTooManyItems = errors.New("list exceeds max_items")

// ErrInvalidValueType indicates that a variable value type does not match the declared type.
var ErrInvalidValueType = errors.New("variable value type does not match declared type")

// ErrTemplateParseFailed indicates that Go template parsing failed.
var ErrTemplateParseFailed = errors.New("failed to parse template content")

// ErrTemplateRenderFailed indicates that Go template rendering failed.
var ErrTemplateRenderFailed = errors.New("failed to render template")
