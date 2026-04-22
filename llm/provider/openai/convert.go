package openai

import (
	oai "github.com/sashabaranov/go-openai"
	"github.com/ygpkg/yg-go/llm/llmtype"
)

// ─── 请求转换：llmtype → openai ───

// convertRequest 将 llmtype.ChatRequest 转换为 openai.ChatCompletionRequest
// TraceID / TaskID 等业务透传字段不传入大模型 API，转换时丢弃
func convertRequest(req *llmtype.ChatRequest) oai.ChatCompletionRequest {
	oaiReq := oai.ChatCompletionRequest{
		Model:     req.Model,
		MaxTokens: req.MaxTokens,
	}

	if req.Temperature != nil {
		oaiReq.Temperature = *req.Temperature
	}
	if req.TopP != nil {
		oaiReq.TopP = *req.TopP
	}

	oaiReq.Messages = convertMessages(req.Messages)
	oaiReq.Tools = convertTools(req.Tools)
	oaiReq.ToolChoice = req.ToolChoice

	return oaiReq
}

// convertMessages 将 llmtype.Message 列表转换为 openai.ChatCompletionMessage 列表
func convertMessages(messages []llmtype.Message) []oai.ChatCompletionMessage {
	result := make([]oai.ChatCompletionMessage, 0, len(messages))
	for _, msg := range messages {
		oaiMsg := oai.ChatCompletionMessage{
			Role:       string(msg.Role),
			Content:    msg.Content,
			ToolCalls:  convertToolCalls(msg.ToolCalls),
			ToolCallID: msg.ToolCallID,
			Name:       msg.Name,
		}

		if len(msg.Parts) > 0 {
			oaiMsg.Content = ""
			oaiMsg.MultiContent = convertParts(msg.Parts)
		}

		result = append(result, oaiMsg)
	}
	return result
}

// convertParts 将 llmtype.Part 列表转换为 openai.ChatMessagePart 列表
func convertParts(parts []llmtype.Part) []oai.ChatMessagePart {
	result := make([]oai.ChatMessagePart, 0, len(parts))
	for _, part := range parts {
		switch p := part.(type) {
		case *llmtype.TextPart:
			result = append(result, oai.ChatMessagePart{
				Type: oai.ChatMessagePartTypeText,
				Text: p.Text,
			})
		case *llmtype.ImageUrlPart:
			result = append(result, oai.ChatMessagePart{
				Type: oai.ChatMessagePartTypeImageURL,
				ImageURL: &oai.ChatMessageImageURL{
					URL:    p.ImageURL.URL,
					Detail: oai.ImageURLDetail(p.ImageURL.Detail),
				},
			})
		}
	}
	return result
}

// convertTools 将 llmtype.Tool 列表转换为 openai.Tool 列表
func convertTools(tools []llmtype.Tool) []oai.Tool {
	if len(tools) == 0 {
		return nil
	}
	result := make([]oai.Tool, 0, len(tools))
	for _, tool := range tools {
		result = append(result, oai.Tool{
			Type: oai.ToolType(tool.Type),
			Function: &oai.FunctionDefinition{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				Parameters:  tool.Function.Parameters,
			},
		})
	}
	return result
}

// convertToolCalls 将 llmtype.ToolCall 列表转换为 openai.ToolCall 列表
func convertToolCalls(toolCalls []llmtype.ToolCall) []oai.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]oai.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		result = append(result, oai.ToolCall{
			ID:   tc.ID,
			Type: oai.ToolType(tc.Type),
			Function: oai.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return result
}

// ─── 响应转换：openai → llmtype ───

// convertResponse 将 openai.ChatCompletionResponse 转换为 llmtype.ChatResponse
func convertResponse(resp oai.ChatCompletionResponse) *llmtype.ChatResponse {
	result := &llmtype.ChatResponse{
		ID:          resp.ID,
		RawResponse: resp,
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		result.Content = choice.Message.Content
		result.FinishReason = llmtype.FinishReason(choice.FinishReason)

		if len(choice.Message.MultiContent) > 0 {
			result.Content = extractTextFromMultiContent(choice.Message.MultiContent)
		}

		result.ToolCalls = convertResponseToolCalls(choice.Message.ToolCalls)
	}

	result.Usage = convertUsage(resp.Usage)

	return result
}

// extractTextFromMultiContent 从 openai MultiContent 中提取纯文本内容拼接
func extractTextFromMultiContent(parts []oai.ChatMessagePart) string {
	var text string
	for _, part := range parts {
		if part.Type == oai.ChatMessagePartTypeText {
			text += part.Text
		}
	}
	return text
}

// convertResponseToolCalls 将 openai.ToolCall 列表转换为 llmtype.ToolCall 列表
func convertResponseToolCalls(toolCalls []oai.ToolCall) []llmtype.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]llmtype.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		result = append(result, llmtype.ToolCall{
			ID:   tc.ID,
			Type: llmtype.ToolType(tc.Type),
			Function: llmtype.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return result
}

// convertUsage 将 openai.Usage 转换为 llmtype.Usage
func convertUsage(usage oai.Usage) llmtype.Usage {
	return llmtype.Usage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
}

// ─── 流式转换：openai stream chunk → llmtype.StreamChunk ───

// convertStreamChunk 将 openai 流式响应增量转换为 llmtype.StreamChunk
func convertStreamChunk(resp oai.ChatCompletionStreamResponse) *llmtype.StreamChunk {
	chunk := &llmtype.StreamChunk{
		ID: resp.ID,
	}

	if resp.Usage != nil {
		chunk.Usage = &llmtype.Usage{
			PromptTokens:     resp.Usage.PromptTokens,
			CompletionTokens: resp.Usage.CompletionTokens,
			TotalTokens:      resp.Usage.TotalTokens,
		}
	}

	if len(resp.Choices) > 0 {
		choice := resp.Choices[0]
		chunk.Content = choice.Delta.Content
		chunk.FinishReason = llmtype.FinishReason(choice.FinishReason)

		if len(choice.Delta.ToolCalls) > 0 {
			chunk.ToolCalls = convertStreamToolCalls(choice.Delta.ToolCalls)
		}
	}

	return chunk
}

// convertStreamToolCalls 将 openai 流式 ToolCall 转换为 llmtype.ToolCall
// 流式增量中 ToolCall 的 Index 字段用于标识同一工具调用的不同增量片段
func convertStreamToolCalls(toolCalls []oai.ToolCall) []llmtype.ToolCall {
	if len(toolCalls) == 0 {
		return nil
	}
	result := make([]llmtype.ToolCall, 0, len(toolCalls))
	for _, tc := range toolCalls {
		result = append(result, llmtype.ToolCall{
			ID:   tc.ID,
			Type: llmtype.ToolType(tc.Type),
			Function: llmtype.FunctionCall{
				Name:      tc.Function.Name,
				Arguments: tc.Function.Arguments,
			},
		})
	}
	return result
}
