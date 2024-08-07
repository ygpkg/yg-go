package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/cache/cachetype"
)

var _ cachetype.Cache = (*Redis)(nil)

// Redis redis cache
type Redis struct {
	ctx  context.Context
	conn redis.UniversalClient
}

// NewCache 实例化
func NewCache(rds *redis.Client) *Redis {
	return &Redis{conn: rds, ctx: context.Background()}
}

// SetConn 设置conn
func (r *Redis) SetConn(conn redis.UniversalClient) {
	r.conn = conn
}

// SetContext 设置redis ctx 参数
func (r *Redis) SetContext(ctx context.Context) {
	r.ctx = ctx
}

// GetContext 获取一个值
func (r *Redis) GetContext(ctx context.Context, key string) interface{} {
	result, err := r.conn.Do(ctx, "GET", key).Result()
	if err != nil {
		return nil
	}
	return result
}

// Get 获取一个值
func (r *Redis) Get(key string, reply interface{}) error {
	data, err := r.conn.Get(r.ctx, key).Result()
	if err != nil {
		return err
	}
	if err = cachetype.Unmarshal([]byte(data), reply); err != nil {
		return err
	}

	return nil
}

// Set 设置一个值
func (r *Redis) Set(key string, val interface{}, timeout time.Duration) (err error) {
	data := cachetype.Marshal(val)
	_, err = r.conn.Set(r.ctx, key, data, timeout).Result()
	if err != nil {
		return err
	}

	return
}

// IsExist 判断key是否存在
func (r *Redis) IsExist(key string) bool {
	i := r.conn.Exists(r.ctx, key).Val()
	if i > 0 {
		return true
	}
	return false
}

// Delete 删除
func (r *Redis) Delete(key string) error {
	if err := r.conn.Del(r.ctx, key).Err(); err != nil {
		return err
	}

	return nil
}
