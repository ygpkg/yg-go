package ssecache

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type redisStorage struct {
	rdb *redis.Client
}

func newRedisStorage(rdb *redis.Client) *redisStorage {
	return &redisStorage{
		rdb: rdb,
	}
}

func (r *redisStorage) WriteMessage(ctx context.Context, key string, msg string, expiration time.Duration) error {
	args := &redis.XAddArgs{
		Stream: key,
		ID:     "*",
		Values: map[string]interface{}{
			"data": msg,
		},
	}

	_, err := r.rdb.XAdd(ctx, args).Result()
	if err != nil {
		return fmt.Errorf("failed to add message to redis stream, err: %v, key:%s, msg:%s", err, key, msg)
	}

	if _, err := r.rdb.Expire(ctx, key, expiration).Result(); err != nil {
		return fmt.Errorf("failed to set expiration for redis stream, err: %v, key:%s", err, key)
	}

	return nil
}

func (r *redisStorage) ReadMessages(ctx context.Context, key string, lastID string) ([]string, error) {
	if lastID == "" {
		lastID = "0"
	}

	count, countErr := r.rdb.XLen(ctx, key).Result()
	if countErr != nil {
		return nil, fmt.Errorf("failed to get stream length, err: %v, key: %s", countErr, key)
	}

	// 读取已有的全部数据
	streams, err := r.rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{key, "0"},
		Count:   count,
		Block:   0,
	}).Result()

	if err != nil {
		return nil, fmt.Errorf("failed to read from redis stream, err: %v, key: %s", err, key)
	}

	var messages []string
	for _, stream := range streams {
		for _, msg := range stream.Messages {
			if data, ok := msg.Values["data"].(string); ok {
				messages = append(messages, data)
			}
		}
	}

	return messages, nil
}

func (r *redisStorage) SetStopSignal(ctx context.Context, key string) error {
	_, err := r.rdb.Set(ctx, key, "stop_signal", defaultExpiration*2).Result()
	if err != nil {
		return fmt.Errorf("failed to set stop signal, err: %v, key:%s", err, key)
	}
	return nil
}

func (r *redisStorage) GetStopSignal(ctx context.Context, key string) (bool, error) {
	count, err := r.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to get stop signal, err: %v, key:%s", err, key)
	}
	return count > 0, nil
}

func (r *redisStorage) RemoveStopSignal(ctx context.Context, key string) error {
	_, err := r.rdb.Del(ctx, key).Result()
	if err != nil {
		return fmt.Errorf("failed to remove stop signal, err: %v, key:%s", err, key)
	}
	return nil
}
