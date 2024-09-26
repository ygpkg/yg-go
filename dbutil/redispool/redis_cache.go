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

func (c *Cache) SetNX(key, data string, expired time.Duration) (bool, error) {
	return c.client.SetNX(context.Background(), key, data, expired).Result()
}

func (c *Cache) Del(key string) error {
	return c.client.Del(context.Background(), key).Err()
}

func (c *Cache) LPush(key string, values ...string) error {
	return c.client.LPush(context.Background(), key, values).Err()
}

func (c *Cache) RPop(key string) (string, error) {
	result, err := c.client.RPop(context.Background(), key).Result()
	if err != nil {
		logs.Errorf("redis_cache call RPop failed,err=%v", err)
		return "", err
	}
	return result, nil
}

func (c *Cache) RPopLPush(src, dst string) (string, error) {
	result, err := c.client.RPopLPush(context.Background(), src, dst).Result()
	if err != nil {
		logs.Errorf("redis_cache call RPopLPush failed,err=%v", err)
		return "", err
	}
	return result, nil
}

func (c *Cache) LRem(key string, count int64, value string) (int64, error) {
	result, err := c.client.LRem(context.Background(), key, count, value).Result()
	if err != nil {
		logs.Errorf("redis_cache call RPopLPush failed,err=%v", err)
		return 0, err
	}
	return result, nil
}

func (c *Cache) ZAdd(key string, members ...redis.Z) (int64, error) {
	result, err := c.client.ZAdd(context.Background(), key, members...).Result()
	if err != nil {
		logs.Errorf("redis_cache call ZAdd failed,err=%v", err)
		return 0, err
	}
	return result, nil
}

func (c *Cache) ZCount(key string, min, max string) (int64, error) {
	result, err := c.client.ZCount(context.Background(), key, min, max).Result()
	if err != nil {
		logs.Errorf("redis_cache call ZCount failed,err=%v", err)
		return 0, err
	}
	return result, nil
}

func (c *Cache) ZIncrBy(key string, increment float64, member string) (float64, error) {
	result, err := c.client.ZIncrBy(context.Background(), key, increment, member).Result()
	if err != nil {
		logs.Errorf("redis_cache call ZIncrBy failed,err=%v", err)
		return 0, err
	}
	return result, nil
}

func (c *Cache) ZRangeWithScores(key string, start, stop int64) ([]redis.Z, error) {
	result, err := c.client.ZRangeWithScores(context.Background(), key, start, stop).Result()
	if err != nil {
		logs.Errorf("redis_cache call ZRangeWithScores failed,err=%v", err)
		return nil, err
	}
	return result, nil
}

func (c *Cache) ZRange(key string, start, stop int64) ([]string, error) {
	result, err := c.client.ZRange(context.Background(), key, start, stop).Result()
	if err != nil {
		logs.Errorf("redis_cache call ZRange failed,err=%v", err)
		return []string{}, err
	}
	return result, nil
}

func (c *Cache) ZRangeByScore(key string, by *redis.ZRangeBy) ([]string, error) {
	result, err := c.client.ZRangeByScore(context.Background(), key, by).Result()
	if err != nil {
		logs.Errorf("redis_cache call ZRangeByScore failed,err=%v", err)
		return []string{}, err
	}
	return result, nil
}

func (c *Cache) ZRangeByScoreWithScores(key string, by *redis.ZRangeBy) ([]redis.Z, error) {
	result, err := c.client.ZRangeByScoreWithScores(context.Background(), key, by).Result()
	if err != nil {
		logs.Errorf("redis_cache call ZRangeByScoreWithScores failed,err=%v", err)
		return nil, err
	}
	return result, nil
}

func (c *Cache) ZRem(key string, member ...interface{}) (int64, error) {
	result, err := c.client.ZRem(context.Background(), key, member...).Result()
	if err != nil {
		logs.Errorf("redis_cache call ZRangeByScoreWithScores failed,err=%v", err)
		return 0, err
	}
	return result, nil
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

func LPush(key string, values ...string) error {
	cache := CacheInstance()
	return cache.LPush(key, values...)
}

// RPop if key not exist in redis,return redis.Nil error
func RPop(key string) (string, error) {
	cache := CacheInstance()
	return cache.RPop(key)
}

// RPopLPush if key not exist in redis,return redis.Nil error
func RPopLPush(src, dst string) (string, error) {
	cache := CacheInstance()
	return cache.RPopLPush(src, dst)
}

// LRem ... 删除列表指定数量的value，返回删除的数量，为0则该值不存在
func LRem(key string, count int64, value string) (int64, error) {
	cache := CacheInstance()
	return cache.LRem(key, count, value)
}

func SetNX(key, data string, expired time.Duration) (bool, error) {
	cache := CacheInstance()
	return cache.SetNX(key, data, expired)
}

func ZAdd(key string, members ...redis.Z) (int64, error) {
	cache := CacheInstance()
	return cache.ZAdd(key, members...)
}

func ZCount(key string, min, max string) (int64, error) {
	cache := CacheInstance()
	return cache.ZCount(key, min, max)
}

func ZIncrBy(key string, increment float64, member string) (float64, error) {
	cache := CacheInstance()
	return cache.ZIncrBy(key, increment, member)
}

func ZRangeWithScores(key string, start, stop int64) ([]redis.Z, error) {
	cache := CacheInstance()
	return cache.ZRangeWithScores(key, start, stop)
}

func ZRange(key string, start, stop int64) ([]string, error) {
	cache := CacheInstance()
	return cache.ZRange(key, start, stop)
}

func ZRangeByScore(key string, by *redis.ZRangeBy) ([]string, error) {
	cache := CacheInstance()
	return cache.ZRangeByScore(key, by)
}

func ZRangeByScoreWithScores(key string, by *redis.ZRangeBy) ([]redis.Z, error) {
	cache := CacheInstance()
	return cache.ZRangeByScoreWithScores(key, by)
}

func ZRem(key string, member ...interface{}) (int64, error) {
	cache := CacheInstance()
	return cache.ZRem(key)
}

// GetLock 获取锁
func GetLock(key string, expired time.Duration) bool {
	cache := CacheInstance()
	result, err := cache.client.SetNX(context.Background(), key, 1, expired).Result()
	if err != nil {
		logs.Errorf("redis_cache call GetLock failed,err=%v", err)
		return false
	}
	return result
}

// Lock 释放锁 key: 锁的key expired: 锁的超时时间
func Lock(key string, expired time.Duration) error {
	for i := 0; i < 100; i++ {
		if GetLock(key, expired) {
			return nil
		}
		time.Sleep(time.Millisecond * 10)
	}
	return fmt.Errorf("lock failed")
}

// UnLock 释放锁
func UnLock(key string) error {
	cache := CacheInstance()
	_, err := cache.client.Del(context.Background(), key).Result()
	if err != nil {
		logs.Errorf("redis_cache call UnLock failed,err=%v", err)
		return err
	}
	return nil
}

// LockWithTimeout 获取锁
// key: 锁的key
// expired: 锁的超时时间
// interval: 获取锁的间隔时间
// timeout: 获取锁的超时时间
func LockWithTimeout(key string, expired, interval, timeout time.Duration) bool {
	start := time.Now()
	for {
		if GetLock(key, expired) {
			return true
		}
		if time.Since(start) > timeout {
			return false
		}
		time.Sleep(interval)
	}
}
