package ssecache

import (
	"net/http"
	"time"
)

// 使用示例
func ExampleUsage() {
	// 1. 单节点部署 - 基于内存
	sseClient := NewSSEClient()

	// 2. 单节点部署 - 消息存储使用Redis，停止信号使用内存
	sseClientRedis := NewSSEClient(&Options{
		RedisAddr:         "localhost:6379",
		RedisPassword:     "",
		RedisDB:           0,
		BufferSize:        100,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		EnableDistributed: false, // 单节点模式
	})

	// 3. 多节点部署 - 消息存储和停止信号都使用Redis
	sseClientDistributed := NewSSEClient(&Options{
		RedisAddr:         "localhost:6379",
		RedisPassword:     "",
		RedisDB:           0,
		BufferSize:        100,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		EnableDistributed: true, // 分布式模式
	})

	defer sseClient.Close()
	defer sseClientRedis.Close()
	defer sseClientDistributed.Close()

	// 创建Gin路由
	router := gin.Default()

	// SSE连接端点
	router.GET("/stream", HeadersMiddleware(), sseClient.SSEHandler())

	// 发送消息API
	router.POST("/send", func(c *gin.Context) {
		var req struct {
			StreamID string      `json:"stream_id"`
			Type     string      `json:"type"`
			Data     interface{} `json:"data"`
		}

		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}

		msg := Message{
			Type: req.Type,
			Data: req.Data,
		}

		if err := sseClient.WriteMessage(req.StreamID, msg); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{"status": "sent"})
	})

	// 停止写入API (用于大模型停止回答等场景)
	router.POST("/stop/write/:stream_id", func(c *gin.Context) {
		streamID := c.Param("stream_id")
		if err := sseClient.StopWrite(streamID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "write stopped"})
	})

	// 停止读取API
	router.POST("/stop/read/:stream_id", func(c *gin.Context) {
		streamID := c.Param("stream_id")
		if err := sseClient.StopRead(streamID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "read stopped"})
	})

	// 停止所有操作API
	router.POST("/stop/all/:stream_id", func(c *gin.Context) {
		streamID := c.Param("stream_id")
		if err := sseClient.StopAll(streamID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "all operations stopped"})
	})

	// 恢复写入API
	router.POST("/resume/write/:stream_id", func(c *gin.Context) {
		streamID := c.Param("stream_id")
		if err := sseClient.ResumeWrite(streamID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "write resumed"})
	})

	// 恢复读取API
	router.POST("/resume/read/:stream_id", func(c *gin.Context) {
		streamID := c.Param("stream_id")
		if err := sseClient.ResumeRead(streamID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "read resumed"})
	})

	router.Run(":8080")
}
