package sseclient

import (
	"context"
	"fmt"
	"time"

	"github.com/go-redis/redis/v8"
)

type redisCache struct {
	rdb *redis.Client
}

func newRedisCache(rdb *redis.Client) *redisCache {
	return &redisCache{
		rdb: rdb,
	}
}

func (r *redisCache) WriteMessage(ctx context.Context, key, msg string, expiration time.Duration) error {
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

func (r *redisCache) ReadMessages(ctx context.Context, key string) (string, []string, error) {

	// 获取最新的 id
	latestResList, getLatestErr := r.rdb.XRevRangeN(ctx, key, "+", "-", 1).Result()
	if getLatestErr != nil {
		return "", nil, fmt.Errorf("failed to get latest id, err: %v, key: %s", getLatestErr, key)
	}
	if len(latestResList) == 0 {
		return "", nil, nil
	}
	latestID := latestResList[0].ID

	// 读取指定 id 之前的数据，包含 latest id
	resList, getListErr := r.rdb.XRange(ctx, key, "-", latestID).Result()
	if getListErr != nil {
		return "", nil, fmt.Errorf("failed to read from redis stream, err: %v, key: %s", getListErr, key)
	}

	var messages []string
	for _, v := range resList {
		if dataVal, ok := v.Values["data"]; ok {
			if str, ok := dataVal.(string); ok {
				messages = append(messages, str)
			}
		}
	}

	return latestID, messages, nil
}

func (r *redisCache) ReadAfterID(ctx context.Context, key, id string) (string, string, error) {
	res, err := r.rdb.XRead(ctx, &redis.XReadArgs{
		Streams: []string{key, id},
		Block:   time.Millisecond * 200,
		Count:   1,
	}).Result()
	if err != nil {
		if err == redis.Nil {
			return "", "", nil
		}
		return "", "", fmt.Errorf("failed to read from redis stream, err: %v, key: %s, id:%s", err, key, id)
	}
	var msg string
	var msgID string
	for _, v := range res {
		for _, msgVal := range v.Messages {
			msgID = msgVal.ID
			if dataVal, ok := msgVal.Values["data"]; ok {
				if str, ok := dataVal.(string); ok {
					msg = str
				}
			}
		}
	}
	return msgID, msg, nil
}

func (r *redisCache) Set(ctx context.Context, key string, expiration time.Duration) error {
	_, err := r.rdb.Set(ctx, key, "stop_signal", expiration).Result()
	if err != nil {
		return fmt.Errorf("failed to set stop signal, err: %v, key:%s", err, key)
	}
	return nil
}

func (r *redisCache) Exist(ctx context.Context, key string) (bool, error) {
	count, err := r.rdb.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to get stop signal, err: %v, key:%s", err, key)
	}
	return count > 0, nil
}

func (r *redisCache) Delete(ctx context.Context, key string) error {
	err := r.rdb.Del(ctx, key).Err()
	if err != nil {
		return fmt.Errorf("failed to delete message, err: %v, key:%s", err, key)
	}
	return nil
}
