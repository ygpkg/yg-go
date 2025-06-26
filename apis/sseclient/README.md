
## 使用说明

### 流式返回
``` go
func StreamChat(ctx *gin.Context) {
	// 设置 SSE 响应头
	sseClient := sseclient.New(sseclient.WithRedisClient(rdb))
	sseClient.SetHeaders(ctx.Writer)

	question := "世界上海拔排名前十的山峰"
	sysPrompt := "请简单给出排名即可"
	messages := []openai.ChatCompletionMessageParamUnion{
		openai.SystemMessage(sysPrompt),
		openai.UserMessage(question),
	}

	params := openai.ChatCompletionNewParams{
		Messages: messages,
		Seed:     openai.Int(0),
		Model:    LLMModel,
	}

	// 创建流式请求
	stream := llmClient.Chat.Completions.NewStreaming(ctx, params)
	acc := openai.ChatCompletionAccumulator{}
	ctx.SSEvent("start", "start")
	ctx.Writer.Flush()
	for stream.Next() {
		chunk := stream.Current()

		acc.AddChunk(chunk)

		// 若本轮 chunk 含有 Content 增量，则输出
		if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
			content := chunk.Choices[0].Delta.Content
			stopped, err := sseClient.WriteMessage(ctx, ctx.Writer, question, content)
			if err != nil {
				glog.Errorf(ctx, "[StreamChat] WriteMessage failed: %v", err)
				return
			} else if stopped {
				return
			}
		}

		// 判断是否拒绝生成（如违反政策）
		if refusal, ok := acc.JustFinishedRefusal(); ok {
			ctx.SSEvent("refusal", refusal)
			ctx.Writer.Flush()
		}
	}
}
```

### 停止流写入和返回
``` go
func StopChat(ctx *gin.Context) {
	questionID := "202506261643"
	sseCache := sseclient.New(sseclient.WithRedisClient(rdb))
	if err := sseCache.Stop(ctx, questionID); err != nil {
		glog.Errorf(ctx, "[StopChat] sseCache.Stop failed, err: %v", err)
	}
	glog.Infof(ctx, "[StopChat] completed")
}
```