package sseclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/go-redis/redis/v8"
)

const (
	defaultExpiration = 30 * time.Minute
)

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
		storage = newMemoryCache()
	}

	cache := &SSEClient{
		config:  cfg,
		storage: storage,
		ch:      make(chan string, 100),
	}

	return cache
}

// WriteMessage 写入消息到指定流，返回写入是否被停止：true-被停止，false-未停止
func (s *SSEClient) WriteMessage(ctx context.Context, streamID, msg string) (bool, error) {
	// 检查写入是否被停止
	stopKey := s.buildStopKey(streamID)
	stopped, err := s.storage.GetStopSignal(ctx, stopKey)
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
	// s.ch <- msg
	// 写入消息到 ch，防止 ch 被关闭导致 panic
	select {
	case s.ch <- msg:
	default:
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
	if err := s.storage.SetStopSignal(ctx, stopKey); err != nil {
		return fmt.Errorf("failed to set write stop signal, err: %v, key:%s", err, stopKey)
	}
	writeKey := s.buildWriteKey(streamID)
	if err := s.storage.DeleteMessage(ctx, writeKey); err != nil {
		return fmt.Errorf("failed to delete message, err: %v, key:%s", err, writeKey)
	}
	return nil
}

func (s *SSEClient) GetStopSignal(ctx context.Context, streamID string) (bool, error) {
	key := s.buildStopKey(streamID)
	return s.storage.GetStopSignal(ctx, key)
}

// Close 写入完成时关闭相关资源
func (s *SSEClient) Close() error {
	s.once.Do(func() {
		close(s.ch)
	})
	return nil
}

func (s *SSEClient) buildWriteKey(streamID string) string {
	return fmt.Sprintf("stream_write:%s", streamID)
}

func (s *SSEClient) buildStopKey(streamID string) string {
	return fmt.Sprintf("stream_stop:%s", streamID)
}
