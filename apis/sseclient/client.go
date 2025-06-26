package sseclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	defaultExpiration = 30 * time.Minute

	writeKeyPrefix = "stream_write"
	stopKeyPrefix  = "stream_stop"
)

type StreamErrorHandler func(err error)

var defaultStreamErrorHandler StreamErrorHandler = func(err error) {
}

type StreamHandler func(w io.Writer) bool

type config struct {
	rdb        *redis.Client
	expiration time.Duration
}

type Option interface {
	apply(*config)
}

type configFunc func(*config)

func (f configFunc) apply(cfg *config) {
	f(cfg)
}

func WithRedisClient(rdb *redis.Client) Option {
	return configFunc(func(cfg *config) {
		cfg.rdb = rdb
	})
}

func WithExpiration(expiration time.Duration) Option {
	return configFunc(func(cfg *config) {
		cfg.expiration = expiration
	})
}

// SSEClient SSE客户端管理器
type SSEClient struct {
	// 存储接口
	storage Cache
	ch      chan string
	once    sync.Once
	config  *config
}

// New 创建SSE客户端实例
func New(opts ...Option) *SSEClient {
	cfg := &config{
		expiration: defaultExpiration,
	}
	for _, opt := range opts {
		opt.apply(cfg)
	}

	var storage Cache
	if cfg.rdb != nil {
		storage = newRedisCache(cfg.rdb)
	} else {
		storage = newMemoryCache(writeKeyPrefix, stopKeyPrefix)
	}

	cache := &SSEClient{
		config:  cfg,
		storage: storage,
		ch:      make(chan string, 100),
	}

	return cache
}

func (s *SSEClient) SetHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
}

// WriteMessage 写入消息到指定流，返回写入是否被停止：true-被停止，false-未停止
func (s *SSEClient) WriteMessage(ctx context.Context, writer io.Writer, streamID, msg string) (bool, error) {
	// 检查写入是否被停止
	stopKey := s.buildStopKey(streamID)
	stopped, err := s.storage.Get(ctx, stopKey)
	if err != nil {
		return false, fmt.Errorf("failed to get write stop signal, err: %v, key:%s", err, stopKey)
	}
	if stopped {
		return true, nil
	}

	writeKey := s.buildWriteKey(streamID)
	if err := s.storage.WriteMessage(ctx, writeKey, msg, s.config.expiration); err != nil {
		return false, err
	}

	// 立即写入前端响应流
	if writer != nil {
		if _, err := writer.Write([]byte(msg)); err != nil {
			return false, fmt.Errorf("failed to write to response writer: %w", err)
		}

		// 尝试刷新响应流
		if flusher, ok := writer.(http.Flusher); ok {
			flusher.Flush()
		} else {
			return false, fmt.Errorf("failed to flush response writer, key:%s", writeKey)
		}
	}

	return false, nil
}

// ReadMessages 读取指定流的消息
func (s *SSEClient) ReadMessages(ctx context.Context, streamID string) ([]string, error) {
	key := s.buildWriteKey(streamID)
	return s.storage.ReadMessages(ctx, key)
}

// Stop 停止指定流的写入操作
func (s *SSEClient) Stop(ctx context.Context, streamID string) error {
	stopKey := s.buildStopKey(streamID)
	if err := s.storage.Set(ctx, stopKey, s.config.expiration*2); err != nil {
		return fmt.Errorf("failed to set write stop signal, err: %v, key:%s", err, stopKey)
	}
	writeKey := s.buildWriteKey(streamID)
	if err := s.storage.Delete(ctx, writeKey); err != nil {
		return fmt.Errorf("failed to delete message, err: %v, key:%s", err, writeKey)
	}
	return nil
}

func (s *SSEClient) GetStopSignal(ctx context.Context, streamID string) (bool, error) {
	key := s.buildStopKey(streamID)
	return s.storage.Get(ctx, key)
}

// Close 写入完成时关闭相关资源
func (s *SSEClient) Close(ctx context.Context, streamID string) error {
	s.once.Do(func() {
		close(s.ch)
	})
	key := s.buildWriteKey(streamID)
	if err := s.storage.Delete(ctx, key); err != nil {
		return fmt.Errorf("failed to delete stop signal, err: %v, key:%s", err, key)
	}
	return nil
}

func (s *SSEClient) buildWriteKey(streamID string) string {
	return fmt.Sprintf("%s:%s", writeKeyPrefix, streamID)
}

func (s *SSEClient) buildStopKey(streamID string) string {
	return fmt.Sprintf("%s:%s", stopKeyPrefix, streamID)
}
