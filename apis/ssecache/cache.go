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
	// DeleteMessage 删除消息
	DeleteMessage(ctx context.Context, key string) error
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

// Cache SSE客户端管理器
type Cache struct {
	// 存储接口
	storage Storage
	once    sync.Once
	config  *config
}

// New 创建SSE客户端实例
func New(opts ...Option) *Cache {
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

	client := &Cache{
		config:  cfg,
		storage: storage,
	}

	return client
}

// WriteMessage 写入消息到指定流
func (c *Cache) WriteMessage(ctx context.Context, streamID string, msg string) error {
	// 检查写入是否被停止
	stopKey := c.buildStopKey(streamID)
	stopped, err := c.storage.GetStopSignal(ctx, stopKey)
	if err != nil {
		return fmt.Errorf("failed to get write stop signal, err: %v, key:%s", err, stopKey)
	}
	if stopped {
		return fmt.Errorf("write to stream %s is stopped", streamID)
	}

	writeKey := c.buildWriteKey(streamID)
	if err := c.storage.WriteMessage(ctx, writeKey, msg, c.config.expiration); err != nil {
		return err
	}
	if c.config.ch != nil {
		c.config.ch <- msg
	}

	return nil
}

// ReadMessages 读取指定流的消息
func (c *Cache) ReadMessages(ctx context.Context, streamID string, lastID string) ([]string, error) {
	key := c.buildWriteKey(streamID)
	return c.storage.ReadMessages(ctx, key, lastID)
}

// Stop 停止指定流的写入操作
func (c *Cache) Stop(ctx context.Context, streamID string) error {
	stopKey := c.buildStopKey(streamID)
	if err := c.storage.SetStopSignal(ctx, stopKey); err != nil {
		return fmt.Errorf("failed to set write stop signal, err: %v, key:%s", err, stopKey)
	}
	writeKey := c.buildWriteKey(streamID)
	if err := c.storage.DeleteMessage(ctx, writeKey); err != nil {
		return fmt.Errorf("failed to delete message, err: %v, key:%s", err, writeKey)
	}
	return nil
}

func (c *Cache) GetStopSignal(ctx context.Context, streamID string) (bool, error) {
	key := c.buildStopKey(streamID)
	return c.storage.GetStopSignal(ctx, key)
}

// Close 写入完成时关闭相关资源
func (c *Cache) Close() error {
	c.once.Do(func() {
		close(c.config.ch)
	})
	return nil
}

func (c *Cache) buildWriteKey(streamID string) string {
	return fmt.Sprintf("stream_write:%s", streamID)
}

func (c *Cache) buildStopKey(streamID string) string {
	return fmt.Sprintf("stream_stop:%s", streamID)
}
