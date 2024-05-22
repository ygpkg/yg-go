package redispool

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/logs"
)

type Cache struct {
	client *redis.Client
}

var stdCache *Cache

func InitCache(cli *redis.Client) {
	stdCache = &Cache{client: cli}
}

func CacheInstance() *Cache {
	if stdCache == nil {
		panic(fmt.Errorf("redis cache is nil"))
	}
	return stdCache
}

func Std() *redis.Client {
	return CacheInstance().client
}

func (c *Cache) IsExist(key string) bool {
	result, err := c.client.Exists(context.Background(), key).Result()
	if err != nil {
		logs.Errorf("redis_cache call IsExist failed,err=%v", err)
		return false
	}
	return result > 0
}

func (c *Cache) TTL(key string) time.Duration {
	result, err := c.client.TTL(context.Background(), key).Result()
	if err != nil {
		logs.Errorf("redis_cache call ExpireTime failed,err=%v", err)
		return 0
	}
	return result
}

func (c *Cache) Get(key string) (string, error) {
	result, err := c.client.Get(context.Background(), key).Result()
	if err != nil {
		logs.Errorf("redis_cache call Get failed,err=%v", err)
		return "", err
	}
	return result, nil
}

func (c *Cache) SetEx(key, data string, expired time.Duration) error {
	return c.client.SetEx(context.Background(), key, data, expired).Err()
}

func (c *Cache) Del(key string) error {
	return c.client.Del(context.Background(), key).Err()
}

func SetString(key, value string, timeout time.Duration) error {
	cache := CacheInstance()
	return cache.SetEx(key, value, timeout)
}

func GetString(key string) (string, error) {
	cache := CacheInstance()
	result, err := cache.Get(key)
	if err != nil {
		return "", err
		//if err != redis.Nil {
		//	return "", ere
		//}
		//return "", errors.New("验证码不存在")
	}

	return result, nil
}

func Del(key string) error {
	cache := CacheInstance()
	return cache.Del(key)
}

func IsExistKey(key string) (bool, time.Duration) {
	cache := CacheInstance()
	isExist := cache.IsExist(key)
	if !isExist {
		return false, -2
	}
	ttl := cache.TTL(key)
	return isExist, ttl
}

func SetJSON(key string, v interface{}, expired time.Duration) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return SetString(key, string(data), expired)
}

func GetJSON(key string, v interface{}) error {
	data, err := GetString(key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(data), v)
}
