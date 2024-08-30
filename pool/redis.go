package pool

import (
	"context"
	"encoding/json"

	"github.com/redis/go-redis/v9"
)

var _ Pool = (*RedisPool)(nil)

// RedisPool redis资源池
type RedisPool struct {
	cli        *redis.Client
	ctx        context.Context
	rdsPoolKey string
}

// NewRedisPool 创建一个redis资源池
func NewRedisPool(ctx context.Context, cli *redis.Client, rdsPoolKey string) *RedisPool {
	if ctx == nil {
		ctx = context.Background()
	}
	return &RedisPool{
		ctx:        ctx,
		cli:        cli,
		rdsPoolKey: rdsPoolKey,
	}
}

// Acquire 从资源池中获取一个资源, 返回值为[]byte
func (rp *RedisPool) Acquire() (interface{}, error) {
	rst := rp.cli.LPop(rp.ctx, rp.rdsPoolKey)
	data, err := rst.Bytes()
	if err != nil {
		return nil, err
	}
	return data, nil
}

// Release 释放一个资源到资源池, v为[]byte或string
func (rp *RedisPool) Release(v interface{}) error {
	return rp.cli.RPush(rp.ctx, rp.rdsPoolKey, v).Err()
}

// AcquireDecode 从资源池中获取一个资源, 并json解析到v
func (rp *RedisPool) AcquireDecode(v interface{}) error {
	val, err := rp.Acquire()
	if err != nil {
		return err
	}
	data := val.([]byte)
	return json.Unmarshal(data, v)
}

// ReleaseEncode 释放一个资源到资源池, 并json编码
func (rp *RedisPool) ReleaseEncode(v interface{}) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return rp.Release(data)
}

// AcquireString 从资源池中获取一个资源, 返回值为string
func (rp *RedisPool) AcquireString() (string, error) {
	rst := rp.cli.LPop(rp.ctx, rp.rdsPoolKey)
	data, err := rst.Result()
	if err != nil {
		return "", err
	}
	return data, nil
}

// ReleaseString 释放一个资源到资源池, v为string
func (rp *RedisPool) ReleaseString(v string) error {
	return rp.Release(v)
}

// Clear 清空资源池
func (rp *RedisPool) Clear() {
	rp.cli.Del(rp.ctx, rp.rdsPoolKey)
}
