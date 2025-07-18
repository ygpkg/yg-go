package sseclient

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	defaultExpiration   = 30 * time.Minute
	defaultBlockTimeout = 300 * time.Millisecond
	writeKeyPrefix      = "stream_write"
	stopKeyPrefix       = "stream_stop"
)

type config struct {
	rdb          *redis.Client
	expiration   time.Duration
	blockTimeout time.Duration
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
func WithBlockTimeout(blockTimeout time.Duration) Option {
	return configFunc(func(cfg *config) {
		cfg.blockTimeout = blockTimeout
	})
}

// SSEClient SSE客户端管理器
type SSEClient struct {
	storage Cache // 存储接口
	config  *config
}

// New 创建SSE客户端实例
func New(opts ...Option) *SSEClient {
	cfg := &config{
		expiration:   defaultExpiration,
		blockTimeout: defaultBlockTimeout,
	}
	for _, opt := range opts {
		opt.apply(cfg)
	}

	var storage Cache
	if cfg.rdb != nil {
		storage = newRedisCache(cfg.rdb, cfg.blockTimeout)
	} else {
		storage = newMemoryCache(writeKeyPrefix, stopKeyPrefix)
	}

	cache := &SSEClient{
		config:  cfg,
		storage: storage,
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
	stopped, err := s.storage.Exist(ctx, stopKey)
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

	if err := s.SendEvent(writer, msg); err != nil {
		return false, err
	}

	return false, nil
}

func (s *SSEClient) SendEvent(writer io.Writer, msg string) error {
	if writer == nil {
		return fmt.Errorf("response writer is nil")
	}
	if _, err := writer.Write([]byte(msg)); err != nil {
		return fmt.Errorf("failed to write to response writer: %v", err)
	}
	if flusher, ok := writer.(http.Flusher); ok {
		flusher.Flush()
	} else {
		return fmt.Errorf("failed to flush response writer, msg:%s", msg)
	}
	return nil
}

// ReadMessages 读取指定流的消息
func (s *SSEClient) ReadMessages(ctx context.Context, streamID string) (string, []string, error) {
	key := s.buildWriteKey(streamID)
	return s.storage.ReadMessages(ctx, key)
}

// BlockRead 阻塞读取指定流的消息，返回 bool 表示是否读取结束，true-读取结束，false-未结束
func (s *SSEClient) BlockRead(ctx context.Context, writer io.Writer, streamID string, latestID string) (bool, int, error) {
	writeKey := s.buildWriteKey(streamID)
	stopKey := s.buildStopKey(streamID)
	var timeoutCount atomic.Int32
	var affectedRows atomic.Int32
	maxTimeout := 3
	nextID := latestID
	for {
		stopped, err := s.storage.Exist(ctx, stopKey)
		if err != nil {
			return false, 0, fmt.Errorf("failed to get write stop signal, err: %v, key:%s", err, stopKey)
		}
		if stopped {
			return true, int(affectedRows.Load()), nil
		}

		if timeoutCount.Load() >= int32(maxTimeout) {
			return true, int(affectedRows.Load()), nil
		}
		select {
		case <-ctx.Done():
			return true, int(affectedRows.Load()), nil
		default:

			msgID, msg, err := s.storage.ReadAfterID(ctx, writeKey, nextID)
			if err != nil {
				return false, int(affectedRows.Load()), err
			}
			if msgID == "" {
				timeoutCount.Add(1)
				continue
			}

			if err := s.SendEvent(writer, msg); err != nil {
				return false, int(affectedRows.Load()), err
			}
			nextID = msgID
			affectedRows.Add(1)
			timeoutCount.Store(0)
		}
	}
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
	return s.storage.Exist(ctx, key)
}

// Close 写入完成时关闭相关资源
func (s *SSEClient) Close(ctx context.Context, streamID string) error {
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
