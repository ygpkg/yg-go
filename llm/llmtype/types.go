package llmtype

import (
	"encoding/json"
	"fmt"
)

// ─── 角色枚举 ───

// Role 消息角色类型
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
	RoleTool      Role = "tool"
)

// ─── 多模态内容 ───

// PartType 多模态内容单元类型
type PartType string

const (
	PartTypeText     PartType = "text"
	PartTypeImageURL PartType = "image_url"
)

// Part 多模态内容单元接口，业务侧通过 Type() 区分 TextPart 和 ImageUrlPart
type Part interface {
	Type() PartType
}

// TextPart 纯文本内容单元
type TextPart struct {
	Text string `json:"text"`
}

func (p *TextPart) Type() PartType { return PartTypeText }

// ImageUrlPart 图片内容单元，支持 URL 和 Base64 两种来源
// ImageURL.URL 可以为普通图片 URL 或 data:image/png;base64,... 格式的 Base64 字符串
type ImageUrlPart struct {
	PartType PartType     `json:"type"`
	ImageURL ImageURLData `json:"image_url"`
}

// ImageURLData 图片 URL 详细数据，包含地址和分辨率偏好
type ImageURLData struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

func (p *ImageUrlPart) Type() PartType { return PartTypeImageURL }

// ─── 消息结构体 ───

// Message 消息结构体，兼容纯文本和多模态内容
// 纯文本消息：设置 Content 字段即可
// 多模态消息：设置 Parts 字段（与 Content 互斥）
// 自定义 JSON 序列化：Parts 非空时 content 序列化为数组，否则序列化为字符串
type Message struct {
	Role       Role       `json:"role"`
	Content    string     `json:"content,omitempty"`
	Parts      []Part     `json:"parts,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	Name       string     `json:"name,omitempty"`
}

// MarshalJSON 自定义序列化：当 Parts 非空时，content 字段输出为 Part 数组格式
func (m Message) MarshalJSON() ([]byte, error) {
	if len(m.Parts) > 0 {
		rawParts := make([]json.RawMessage, 0, len(m.Parts))
		for _, part := range m.Parts {
			b, err := json.Marshal(part)
			if err != nil {
				return nil, fmt.Errorf("marshal part failed: %w", err)
			}
			rawParts = append(rawParts, b)
		}
		aux := struct {
			Role       string          `json:"role"`
			Content    json.RawMessage `json:"content"`
			ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
			ToolCallID string          `json:"tool_call_id,omitempty"`
			Name       string          `json:"name,omitempty"`
		}{
			Role:       string(m.Role),
			Content:    json.RawMessage(bytesJoin(rawParts)),
			ToolCalls:  m.ToolCalls,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
		}
		return json.Marshal(aux)
	}

	aux := struct {
		Role       string     `json:"role"`
		Content    string     `json:"content,omitempty"`
		ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
		ToolCallID string     `json:"tool_call_id,omitempty"`
		Name       string     `json:"name,omitempty"`
	}{
		Role:       string(m.Role),
		Content:    m.Content,
		ToolCalls:  m.ToolCalls,
		ToolCallID: m.ToolCallID,
		Name:       m.Name,
	}
	return json.Marshal(aux)
}

// UnmarshalJSON 自定义反序列化：content 字段可以为字符串或数组，数组时填充 Parts
func (m *Message) UnmarshalJSON(data []byte) error {
	aux := struct {
		Role       string          `json:"role"`
		Content    json.RawMessage `json:"content"`
		ToolCalls  []ToolCall      `json:"tool_calls,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
		Name       string          `json:"name,omitempty"`
	}{}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	m.Role = Role(aux.Role)
	m.ToolCalls = aux.ToolCalls
	m.ToolCallID = aux.ToolCallID
	m.Name = aux.Name

	if len(aux.Content) == 0 {
		return nil
	}

	if aux.Content[0] == '"' {
		var s string
		if err := json.Unmarshal(aux.Content, &s); err != nil {
			return err
		}
		m.Content = s
		return nil
	}

	if aux.Content[0] == '[' {
		var rawParts []json.RawMessage
		if err := json.Unmarshal(aux.Content, &rawParts); err != nil {
			return err
		}
		m.Parts = make([]Part, 0, len(rawParts))
		for _, raw := range rawParts {
			typed := struct {
				Type PartType `json:"type"`
			}{}
			if err := json.Unmarshal(raw, &typed); err != nil {
				return err
			}
			switch typed.Type {
			case PartTypeText:
				tp := &TextPart{}
				if err := json.Unmarshal(raw, tp); err != nil {
					return err
				}
				m.Parts = append(m.Parts, tp)
			case PartTypeImageURL:
				ip := &ImageUrlPart{}
				if err := json.Unmarshal(raw, ip); err != nil {
					return err
				}
				m.Parts = append(m.Parts, ip)
			default:
				return fmt.Errorf("unknown part type: %s", typed.Type)
			}
		}
		return nil
	}

	var s string
	if err := json.Unmarshal(aux.Content, &s); err != nil {
		return err
	}
	m.Content = s
	return nil
}

// bytesJoin 将多个 json.RawMessage 拼接为 JSON 数组字节数
func bytesJoin(parts []json.RawMessage) []byte {
	buf := []byte{'['}
	for i, p := range parts {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = append(buf, p...)
	}
	buf = append(buf, ']')
	return buf
}

// ─── Tool Calling 抽象 ───

// ToolType 工具类型枚举
type ToolType string

const (
	ToolTypeFunction ToolType = "function"
)

// Tool 工具定义，用于在请求中声明可用工具
type Tool struct {
	Type     ToolType           `json:"type"`
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition 函数定义，包含名称、描述和 JSON Schema 参数描述
type FunctionDefinition struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Parameters  any    `json:"parameters,omitempty"`
}

// ToolCall 大模型返回的工具调用，包含调用 ID、类型和函数调用详情
type ToolCall struct {
	ID       string       `json:"id"`
	Type     ToolType     `json:"type"`
	Function FunctionCall `json:"function"`
}

// FunctionCall 函数调用详情，包含函数名和 JSON 格式的参数字符串
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ─── ToolChoice ───

// ToolChoice 工具选择策略，支持以下形式：
// - 字符串："none" / "auto"
// - 结构体：指定特定函数 {"type":"function","function":{"name":"xxx"}}
type ToolChoice any

// ─── FinishReason ───

// FinishReason 响应结束原因枚举
type FinishReason string

const (
	FinishReasonStop      FinishReason = "stop"
	FinishReasonLength    FinishReason = "length"
	FinishReasonToolCalls FinishReason = "tool_calls"
)

// ─── Usage Token 计费 ───

// Usage Token 消耗统计，用于计费追踪
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ─── ChatRequest ───

// ChatRequest 聊天补全请求结构体
// TraceID 和 TaskID 为业务侧透传字段，json:"-" 标签确保不会传入大模型 API
type ChatRequest struct {
	Model       string     `json:"model"`
	Messages    []Message  `json:"messages"`
	Tools       []Tool     `json:"tools,omitempty"`
	ToolChoice  ToolChoice `json:"tool_choice,omitempty"`
	Temperature *float32   `json:"temperature,omitempty"`
	MaxTokens   int        `json:"max_tokens,omitempty"`
	TopP        *float32   `json:"top_p,omitempty"`
	TraceID     string     `json:"-"`
	TaskID      string     `json:"-"`
}

// ─── ChatResponse ───

// ChatResponse 聊天补全响应结构体
// RawResponse 保留底层原始响应体，便于 debug 和计费对账
type ChatResponse struct {
	ID           string       `json:"id,omitempty"`
	Content      string       `json:"content,omitempty"`
	ToolCalls    []ToolCall   `json:"tool_calls,omitempty"`
	FinishReason FinishReason `json:"finish_reason"`
	Usage        Usage        `json:"usage"`
	RawResponse  any          `json:"raw_response,omitempty"`
}

// ─── 辅助函数 ───

// Float32Ptr 返回 float32 的指针，用于 Temperature/TopP 等可选字段
func Float32Ptr(v float32) *float32 {
	return &v
}

// IntPtr 返回 int 的指针，用于可选整型字段
func IntPtr(v int) *int {
	return &v
}
