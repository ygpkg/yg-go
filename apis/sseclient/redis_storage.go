package sseclient

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

func (r *redisStorage) WriteMessage(ctx context.Context, key string, msg string) error {
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

	return nil
}

func (r *redisStorage) ReadMessages(ctx context.Context, key string, lastID string) ([]string, error) {
	if lastID == "" {
		lastID = "0"
	}

	streams, err := r.rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{key},
		Count:   100,
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
	_, err := r.rdb.Set(ctx, "stop:"+key, "1", time.Minute*5).Result()
	if err != nil {
		return fmt.Errorf("failed to set stop signal, err: %v, key:%s", err, key)
	}
	return nil
}

func (r *redisStorage) GetStopSignal(ctx context.Context, key string) (bool, error) {
	count, err := r.rdb.Exists(ctx, "stop:"+key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to get stop signal, err: %v, key:%s", err, key)
	}
	return count > 0, nil
}
