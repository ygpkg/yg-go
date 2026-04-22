package openai

import (
	oai "github.com/sashabaranov/go-openai"
	"github.com/ygpkg/yg-go/llm/llmtype"
)

// handleStream 处理流式响应，将 openai 流式回调分发到 llmtype.StreamHandler
func handleStream(stream *oai.ChatCompletionStream, handler llmtype.StreamHandler) error {
	defer stream.Close()

	// 流式 ToolCall 增量拼接缓存：按 ToolCall.Index 维度缓存 Name 和 Arguments
	toolCallCache := make(map[int]*llmtype.ToolCall)

	for {
		resp, err := stream.Recv()
		if err != nil {
			if isStreamEOF(err) {
				doneChunk := &llmtype.StreamChunk{Done: true}
				if !handler(doneChunk, nil) {
					return nil
				}
				return nil
			}
			cont := handler(nil, err)
			if !cont {
				return nil
			}
			return err
		}

		chunk := convertStreamChunk(resp)

		// 处理流式 ToolCall 增量拼接
		if len(resp.Choices) > 0 && len(resp.Choices[0].Delta.ToolCalls) > 0 {
			chunk.ToolCalls = mergeStreamToolCalls(toolCallCache, resp.Choices[0].Delta.ToolCalls)
		}

		// 流结束时携带 ToolCall 缓存的完整结果
		if len(resp.Choices) > 0 && resp.Choices[0].FinishReason == oai.FinishReasonToolCalls {
			chunk.ToolCalls = flushToolCallCache(toolCallCache)
		}

		if !handler(chunk, nil) {
			return nil
		}
	}
}

// mergeStreamToolCalls 将流式 ToolCall 增量与缓存拼接，返回当前累积的完整 ToolCalls
func mergeStreamToolCalls(cache map[int]*llmtype.ToolCall, deltas []oai.ToolCall) []llmtype.ToolCall {
	for _, delta := range deltas {
		if delta.Index == nil {
			continue
		}
		idx := *delta.Index

		if cache[idx] == nil {
			cache[idx] = &llmtype.ToolCall{
				Type: llmtype.ToolType(delta.Type),
				Function: llmtype.FunctionCall{
					Name:      delta.Function.Name,
					Arguments: delta.Function.Arguments,
				},
			}
			if delta.ID != "" {
				cache[idx].ID = delta.ID
			}
			continue
		}

		if delta.ID != "" {
			cache[idx].ID = delta.ID
		}
		if delta.Function.Name != "" {
			cache[idx].Function.Name += delta.Function.Name
		}
		if delta.Function.Arguments != "" {
			cache[idx].Function.Arguments += delta.Function.Arguments
		}
	}

	result := make([]llmtype.ToolCall, 0, len(cache))
	for _, tc := range cache {
		result = append(result, *tc)
	}
	return result
}

// flushToolCallCache 输出缓存中完整的 ToolCall 结果并清空缓存
func flushToolCallCache(cache map[int]*llmtype.ToolCall) []llmtype.ToolCall {
	result := make([]llmtype.ToolCall, 0, len(cache))
	for _, tc := range cache {
		result = append(result, *tc)
	}
	return result
}
