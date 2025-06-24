package sseclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/go-redis/redis/v8"
)

// Message 表示消息结构
type Message struct {
	ID        string      `json:"id"`
	Type      string      `json:"type"`
	Data      interface{} `json:"data"`
	StreamID  string      `json:"stream_id"`
	Timestamp int64       `json:"timestamp"`
}

// Client 表示SSE客户端连接
type Client struct {
	ID      string
	Channel chan Message
	Ctx     context.Context
	Cancel  context.CancelFunc
}

// MessageStorage 消息存储接口
type MessageStorage interface {
	// WriteMessage 写入消息到指定流
	WriteMessage(ctx context.Context, streamID string, msg Message) error
	// ReadMessages 读取指定流的消息
	ReadMessages(ctx context.Context, streamID string, lastID string) ([]Message, error)
	// Close 关闭存储连接
	Close() error
}

// SignalStorage 停止信号存储接口
type SignalStorage interface {
	// SetStopSignal 设置停止信号
	SetStopSignal(ctx context.Context, key string) error
	// GetStopSignal 获取停止信号状态
	GetStopSignal(ctx context.Context, key string) (bool, error)
	// RemoveStopSignal 移除停止信号
	RemoveStopSignal(ctx context.Context, key string) error
	// Close 关闭存储连接
	Close() error
}

// MemoryMessageStorage 内存消息存储实现
type MemoryMessageStorage struct {
	data map[string][]Message
	mu   sync.RWMutex
}

func NewMemoryMessageStorage() *MemoryMessageStorage {
	return &MemoryMessageStorage{
		data: make(map[string][]Message),
	}
}

func (m *MemoryMessageStorage) WriteMessage(ctx context.Context, streamID string, msg Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[streamID] = append(m.data[streamID], msg)
	return nil
}

func (m *MemoryMessageStorage) ReadMessages(ctx context.Context, streamID string, lastID string) ([]Message, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if messages, exists := m.data[streamID]; exists {
		result := make([]Message, len(messages))
		copy(result, messages)
		return result, nil
	}
	return []Message{}, nil
}

func (m *MemoryMessageStorage) Close() error {
	return nil
}

// RedisMessageStorage Redis消息存储实现（使用Stream）
type RedisMessageStorage struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisMessageStorage(addr, password string, db int) *RedisMessageStorage {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &RedisMessageStorage{
		client: rdb,
		ctx:    context.Background(),
	}
}

func (r *RedisMessageStorage) WriteMessage(ctx context.Context, streamID string, msg Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %v", err)
	}

	args := &redis.XAddArgs{
		Stream: fmt.Sprintf("stream:%s", streamID),
		ID:     "*",
		Values: map[string]interface{}{
			"data": string(data),
		},
	}

	_, err = r.client.XAdd(ctx, args).Result()
	if err != nil {
		return fmt.Errorf("failed to add message to redis stream: %v", err)
	}

	return nil
}

func (r *RedisMessageStorage) ReadMessages(ctx context.Context, streamID string, lastID string) ([]Message, error) {
	if lastID == "" {
		lastID = "0"
	}

	streams, err := r.client.XRead(ctx, &redis.XReadArgs{
		Streams: []string{fmt.Sprintf("stream:%s", streamID), lastID},
		Count:   100,
		Block:   0,
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to read from redis stream: %v", err)
	}

	var messages []Message
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			if data, ok := msg.Values["data"].(string); ok {
				var message Message
				if err := json.Unmarshal([]byte(data), &message); err == nil {
					messages = append(messages, message)
				}
			}
		}
	}

	return messages, nil
}

func (r *RedisMessageStorage) Close() error {
	return r.client.Close()
}

// MemorySignalStorage 内存信号存储实现
type MemorySignalStorage struct {
	signals map[string]bool
	mu      sync.RWMutex
}

func NewMemorySignalStorage() *MemorySignalStorage {
	return &MemorySignalStorage{
		signals: make(map[string]bool),
	}
}

func (m *MemorySignalStorage) SetStopSignal(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signals[key] = true
	return nil
}

func (m *MemorySignalStorage) GetStopSignal(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.signals[key], nil
}

func (m *MemorySignalStorage) RemoveStopSignal(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.signals, key)
	return nil
}

func (m *MemorySignalStorage) Close() error {
	return nil
}

// RedisSignalStorage Redis信号存储实现
type RedisSignalStorage struct {
	client *redis.Client
	ctx    context.Context
}

func NewRedisSignalStorage(addr, password string, db int) *RedisSignalStorage {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	return &RedisSignalStorage{
		client: rdb,
		ctx:    context.Background(),
	}
}

func (r *RedisSignalStorage) SetStopSignal(ctx context.Context, key string) error {
	return r.client.Set(ctx, fmt.Sprintf("signal:%s", key), "1", 24*time.Hour).Err()
}

func (r *RedisSignalStorage) GetStopSignal(ctx context.Context, key string) (bool, error) {
	result, err := r.client.Get(ctx, fmt.Sprintf("signal:%s", key)).Result()
	if err == redis.Nil {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return result == "1", nil
}

func (r *RedisSignalStorage) RemoveStopSignal(ctx context.Context, key string) error {
	return r.client.Del(ctx, fmt.Sprintf("signal:%s", key)).Err()
}

func (r *RedisSignalStorage) Close() error {
	return r.client.Close()
}

// Options SSE客户端配置选项
type Options struct {
	// Redis配置
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// 其他配置
	BufferSize   int
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// 多节点部署配置
	EnableDistributed bool // 是否启用分布式模式（使用Redis存储停止信号）
}

// SSEClient SSE客户端管理器
type SSEClient struct {
	// 存储接口
	messageStorage MessageStorage
	signalStorage  SignalStorage

	// 客户端管理
	clients   map[string]*Client
	clientsMu sync.RWMutex

	// 配置
	options *Options
	ctx     context.Context
}

// NewSSEClient 创建SSE客户端实例
func NewSSEClient(opts ...*Options) *SSEClient {
	var options *Options
	if len(opts) > 0 && opts[0] != nil {
		options = opts[0]
	} else {
		options = &Options{
			BufferSize:        100,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			EnableDistributed: false,
		}
	}

	client := &SSEClient{
		clients: make(map[string]*Client),
		options: options,
		ctx:     context.Background(),
	}

	// 初始化消息存储
	if options.RedisAddr != "" {
		client.messageStorage = NewRedisMessageStorage(options.RedisAddr, options.RedisPassword, options.RedisDB)

		// 测试Redis连接
		if err := client.testRedisConnection(); err != nil {
			log.Printf("Redis connection failed, falling back to memory: %v", err)
			client.messageStorage = NewMemoryMessageStorage()
		}
	} else {
		client.messageStorage = NewMemoryMessageStorage()
	}

	// 初始化信号存储
	if options.EnableDistributed && options.RedisAddr != "" {
		// 分布式模式使用Redis存储停止信号
		client.signalStorage = NewRedisSignalStorage(options.RedisAddr, options.RedisPassword, options.RedisDB)

		// 测试Redis连接
		if err := client.testRedisConnection(); err != nil {
			log.Printf("Redis signal storage failed, falling back to memory: %v", err)
			client.signalStorage = NewMemorySignalStorage()
		}
	} else {
		// 单节点模式使用内存存储停止信号
		client.signalStorage = NewMemorySignalStorage()
	}

	return client
}

// testRedisConnection 测试Redis连接
func (s *SSEClient) testRedisConnection() error {
	if redisStorage, ok := s.messageStorage.(*RedisMessageStorage); ok {
		return redisStorage.client.Ping(s.ctx).Err()
	}
	return nil
}

// WriteMessage 写入消息到指定流
func (s *SSEClient) WriteMessage(streamID string, msg Message) error {
	// 检查写入是否被停止
	stopped, err := s.signalStorage.GetStopSignal(s.ctx, fmt.Sprintf("write:%s", streamID))
	if err != nil {
		log.Printf("Failed to check write stop signal: %v", err)
	}
	if stopped {
		return fmt.Errorf("write operation stopped for stream: %s", streamID)
	}

	// 设置消息属性
	if msg.ID == "" {
		msg.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if msg.Timestamp == 0 {
		msg.Timestamp = time.Now().Unix()
	}
	msg.StreamID = streamID

	// 写入消息
	ctx, cancel := context.WithTimeout(s.ctx, s.options.WriteTimeout)
	defer cancel()

	if err := s.messageStorage.WriteMessage(ctx, streamID, msg); err != nil {
		return err
	}

	// 通知客户端有新消息
	s.notifyClients(streamID, msg)

	return nil
}

// ReadMessages 读取指定流的消息
func (s *SSEClient) ReadMessages(streamID string, lastID string) ([]Message, error) {
	// 检查读取是否被停止
	stopped, err := s.signalStorage.GetStopSignal(s.ctx, fmt.Sprintf("read:%s", streamID))
	if err != nil {
		log.Printf("Failed to check read stop signal: %v", err)
	}
	if stopped {
		return nil, fmt.Errorf("read operation stopped for stream: %s", streamID)
	}

	ctx, cancel := context.WithTimeout(s.ctx, s.options.ReadTimeout)
	defer cancel()

	return s.messageStorage.ReadMessages(ctx, streamID, lastID)
}

// StopWrite 停止指定流的写入操作
func (s *SSEClient) StopWrite(streamID string) error {
	key := fmt.Sprintf("write:%s", streamID)
	if err := s.signalStorage.SetStopSignal(s.ctx, key); err != nil {
		return fmt.Errorf("failed to set write stop signal: %v", err)
	}

	log.Printf("Stopped write operations for stream: %s", streamID)
	return nil
}

// StopRead 停止指定流的读取操作
func (s *SSEClient) StopRead(streamID string) error {
	key := fmt.Sprintf("read:%s", streamID)
	if err := s.signalStorage.SetStopSignal(s.ctx, key); err != nil {
		return fmt.Errorf("failed to set read stop signal: %v", err)
	}

	log.Printf("Stopped read operations for stream: %s", streamID)
	return nil
}

// StopAll 停止指定流的所有操作（读写）
func (s *SSEClient) StopAll(streamID string) error {
	if err := s.StopWrite(streamID); err != nil {
		return err
	}
	if err := s.StopRead(streamID); err != nil {
		return err
	}

	// 停止流本身
	key := streamID
	if err := s.signalStorage.SetStopSignal(s.ctx, key); err != nil {
		return fmt.Errorf("failed to set stream stop signal: %v", err)
	}

	log.Printf("Stopped all operations for stream: %s", streamID)
	return nil
}

// ResumeWrite 恢复指定流的写入操作
func (s *SSEClient) ResumeWrite(streamID string) error {
	key := fmt.Sprintf("write:%s", streamID)
	if err := s.signalStorage.RemoveStopSignal(s.ctx, key); err != nil {
		return fmt.Errorf("failed to remove write stop signal: %v", err)
	}

	log.Printf("Resumed write operations for stream: %s", streamID)
	return nil
}

// ResumeRead 恢复指定流的读取操作
func (s *SSEClient) ResumeRead(streamID string) error {
	key := fmt.Sprintf("read:%s", streamID)
	if err := s.signalStorage.RemoveStopSignal(s.ctx, key); err != nil {
		return fmt.Errorf("failed to remove read stop signal: %v", err)
	}

	log.Printf("Resumed read operations for stream: %s", streamID)
	return nil
}

// ResumeAll 恢复指定流的所有操作
func (s *SSEClient) ResumeAll(streamID string) error {
	if err := s.ResumeWrite(streamID); err != nil {
		return err
	}
	if err := s.ResumeRead(streamID); err != nil {
		return err
	}

	// 恢复流本身
	key := streamID
	if err := s.signalStorage.RemoveStopSignal(s.ctx, key); err != nil {
		return fmt.Errorf("failed to remove stream stop signal: %v", err)
	}

	log.Printf("Resumed all operations for stream: %s", streamID)
	return nil
}

// addClient 添加客户端连接
func (s *SSEClient) addClient(clientID string, streamID string) *Client {
	ctx, cancel := context.WithCancel(s.ctx)

	client := &Client{
		ID:      clientID,
		Channel: make(chan Message, s.options.BufferSize),
		Ctx:     ctx,
		Cancel:  cancel,
	}

	s.clientsMu.Lock()
	s.clients[clientID] = client
	s.clientsMu.Unlock()

	log.Printf("Added client %s for stream %s", clientID, streamID)
	return client
}

// removeClient 移除客户端连接
func (s *SSEClient) removeClient(clientID string) {
	s.clientsMu.Lock()
	defer s.clientsMu.Unlock()

	if client, exists := s.clients[clientID]; exists {
		client.Cancel()
		close(client.Channel)
		delete(s.clients, clientID)
		log.Printf("Removed client %s", clientID)
	}
}

// notifyClients 通知相关客户端有新消息
func (s *SSEClient) notifyClients(streamID string, msg Message) {
	s.clientsMu.RLock()
	defer s.clientsMu.RUnlock()

	for _, client := range s.clients {
		select {
		case client.Channel <- msg:
		case <-client.Ctx.Done():
			go s.removeClient(client.ID)
		default:
			// 客户端缓冲区满，跳过
		}
	}
}

// HeadersMiddleware SSE头部中间件
func HeadersMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Content-Type", "text/event-stream")
		c.Writer.Header().Set("Cache-Control", "no-cache")
		c.Writer.Header().Set("Connection", "keep-alive")
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		c.Next()
	}
}

// SSEHandler SSE处理函数
func (s *SSEClient) SSEHandler() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := c.Query("client_id")
		streamID := c.Query("stream_id")
		lastID := c.Query("last_id")

		if clientID == "" || streamID == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "client_id and stream_id are required"})
			return
		}

		client := s.addClient(clientID, streamID)
		defer s.removeClient(clientID)

		// 发送历史消息
		if messages, err := s.ReadMessages(streamID, lastID); err == nil {
			for _, msg := range messages {
				select {
				case client.Channel <- msg:
				case <-client.Ctx.Done():
					return
				}
			}
		}

		c.Stream(func(w io.Writer) bool {
			select {
			case msg := <-client.Channel:
				// 检查读取是否被停止
				stopped, err := s.signalStorage.GetStopSignal(s.ctx, fmt.Sprintf("read:%s", streamID))
				if err != nil {
					log.Printf("Failed to check read stop signal: %v", err)
				}
				if stopped {
					return false
				}

				data, _ := json.Marshal(msg)
				c.SSEvent("message", string(data))
				return true
			case <-client.Ctx.Done():
				return false
			case <-c.Request.Context().Done():
				return false
			}
		})
	}
}

// Close 关闭SSE客户端管理器
func (s *SSEClient) Close() error {
	// 关闭所有客户端
	s.clientsMu.RLock()
	clientIDs := make([]string, 0, len(s.clients))
	for id := range s.clients {
		clientIDs = append(clientIDs, id)
	}
	s.clientsMu.RUnlock()

	for _, id := range clientIDs {
		s.removeClient(id)
	}

	// 关闭存储连接
	var err1, err2 error
	if s.messageStorage != nil {
		err1 = s.messageStorage.Close()
	}
	if s.signalStorage != nil {
		err2 = s.signalStorage.Close()
	}

	if err1 != nil {
		return err1
	}
	return err2
}

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
