
## 使用说明

### 流式返回
``` go
func Chat(ctx *gin.Context) {
	questionID := "202506261643"

    // 实例化 client
	sseClient := sseclient.New(sseclient.WithRedisClient(rdb), sseclient.WithExpiration(time.Second*60))

    // 修改 header
	sseClient.SetHeaders(ctx.Writer)

    // 模拟数据写入
	var writeCount int
	var mu sync.Mutex
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				mu.Lock()
				if writeCount > 100 {
					mu.Unlock()
					return
				}
				msg := time.Now().Format(time.RFC3339)
				if stoped, err := sseClient.WriteMessage(ctx, questionID, msg); err != nil {
					glog.Errorf(ctx, "[Chat] WriteMessage failed: %v", err)
					mu.Unlock()
					return
				} else if stoped {
					mu.Unlock()
					return
				}
				writeCount++
				mu.Unlock()
			}
		}
	}()

    // 流式返回
	errorHandler := func(err error) {
		glog.Errorf(ctx, "[Chat] stream handler failed, err: %v", err)
	}

	clientGone := ctx.Stream(sseClient.GetStreamHandler(ctx, questionID, errorHandler))
    if clientGone {
        glog.Infof(ctx, "[Chat] client gone")
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