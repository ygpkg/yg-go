package llmtype

import "context"

// Client LLM 统一抽象接口，业务侧通过此接口调用大模型，禁止直接 import 任何厂商 SDK
type Client interface {
	// Chat 同步调用大模型，等待完整响应返回
	Chat(ctx context.Context, req *ChatRequest) (*ChatResponse, error)

	// ChatStream 流式调用大模型，通过回调函数逐块处理增量响应
	// handler 返回 true 继续，返回 false 中断流
	ChatStream(ctx context.Context, req *ChatRequest, handler StreamHandler) error
}
