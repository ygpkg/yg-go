package llmtype

// StreamChunk 流式响应的单个增量数据块
type StreamChunk struct {
	ID           string       `json:"id,omitempty"`
	Content      string       `json:"content,omitempty"`
	ToolCalls    []ToolCall   `json:"tool_calls,omitempty"`
	FinishReason FinishReason `json:"finish_reason,omitempty"`
	Usage        *Usage       `json:"usage,omitempty"`
	Done         bool         `json:"done"`
}

// StreamHandler 流式回调函数类型
// 每收到一个 SSE chunk 时调用 handler(chunk, nil)
// 发生错误时调用 handler(nil, err)
// 返回 true 继续消费，返回 false 主动中断流
type StreamHandler func(chunk *StreamChunk, err error) bool
