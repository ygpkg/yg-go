package ssecache

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

type Storage interface {
	// WriteMessage 写入消息到指定流
	WriteMessage(ctx context.Context, key string, msg string, expiration time.Duration) error
	// ReadMessages 读取指定流的消息
	ReadMessages(ctx context.Context, key string, lastID string) ([]string, error)
	// SetStopSignal 设置停止信号
	SetStopSignal(ctx context.Context, key string) error
	// GetStopSignal 获取停止信号状态
	GetStopSignal(ctx context.Context, key string) (bool, error)
}

type config struct {
	rdb        *redis.Client
	ch         chan string
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

func WithChannel(ch chan string) Option {
	return configFunc(func(cfg *config) {
		cfg.ch = ch
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
	storage Storage
	once    sync.Once
	config  *config
}

// NewSSEClient 创建SSE客户端实例
func NewSSEClient(opts ...Option) *SSEClient {
	cfg := &config{
		expiration: defaultExpiration,
	}
	for _, opt := range opts {
		opt.apply(cfg)
	}

	var storage Storage
	if cfg.rdb != nil {
		storage = newRedisStorage(cfg.rdb)
	} else {
		storage = newMemoryStorage()
	}

	client := &SSEClient{
		config:  cfg,
		storage: storage,
	}

	return client
}

// WriteMessage 写入消息到指定流
func (s *SSEClient) WriteMessage(ctx context.Context, streamID string, msg string) error {
	// 检查写入是否被停止
	stopKey := s.buildStopKey(streamID)
	stopped, err := s.storage.GetStopSignal(ctx, stopKey)
	if err != nil {
		return fmt.Errorf("failed to get write stop signal, err: %v, key:%s", err, stopKey)
	}
	if stopped {
		return fmt.Errorf("write to stream %s is stopped", streamID)
	}

	writeKey := s.buildWriteKey(streamID)
	if err := s.storage.WriteMessage(ctx, writeKey, msg); err != nil {
		return err
	}
	if s.config.ch != nil {
		s.config.ch <- msg
	}

	return nil
}

// ReadMessages 读取指定流的消息
func (s *SSEClient) ReadMessages(ctx context.Context, streamID string, lastID string) ([]string, error) {
	key := s.buildWriteKey(streamID)
	return s.storage.ReadMessages(ctx, key, lastID)
}

// Stop 停止指定流的写入操作
func (s *SSEClient) Stop(ctx context.Context, streamID string) error {
	key := s.buildStopKey(streamID)
	if err := s.storage.SetStopSignal(ctx, key); err != nil {
		return fmt.Errorf("failed to set write stop signal, err: %v, key:%s", err, key)
	}
	return nil
}

// Close 写入完成时关闭相关资源
func (s *SSEClient) Close() error {
	s.once.Do(func() {
		close(s.config.ch)
	})
	return nil
}

func (s *SSEClient) buildWriteKey(streamID string) string {
	return fmt.Sprintf("stream_write:%s", streamID)
}

func (s *SSEClient) buildStopKey(streamID string) string {
	return fmt.Sprintf("stream_stop:%s", streamID)
}
