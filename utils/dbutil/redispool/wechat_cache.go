package redispool

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// wechatCache .redis cache
type wechatCache struct {
	ctx  context.Context
	conn redis.UniversalClient
}

// SetConn 设置conn
func (r *wechatCache) SetConn(conn redis.UniversalClient) {
	r.conn = conn
}

// SetRedisCtx 设置redis ctx 参数
func (r *wechatCache) SetRedisCtx(ctx context.Context) {
	r.ctx = ctx
}

// Get 获取一个值
func (r *wechatCache) Get(key string) interface{} {
	return r.GetContext(r.ctx, key)
}

// GetContext 获取一个值
func (r *wechatCache) GetContext(ctx context.Context, key string) interface{} {
	result, err := r.conn.Do(ctx, "GET", key).Result()
	if err != nil {
		return nil
	}
	return result
}

// Set 设置一个值
func (r *wechatCache) Set(key string, val interface{}, timeout time.Duration) error {
	return r.SetContext(r.ctx, key, val, timeout)
}

// SetContext 设置一个值
func (r *wechatCache) SetContext(ctx context.Context, key string, val interface{}, timeout time.Duration) error {
	return r.conn.SetEx(ctx, key, val, timeout).Err()
}

// IsExist 判断key是否存在
func (r *wechatCache) IsExist(key string) bool {
	return r.IsExistContext(r.ctx, key)
}

// IsExistContext 判断key是否存在
func (r *wechatCache) IsExistContext(ctx context.Context, key string) bool {
	result, _ := r.conn.Exists(ctx, key).Result()

	return result > 0
}

// Delete 删除
func (r *wechatCache) Delete(key string) error {
	return r.DeleteContext(r.ctx, key)
}

// DeleteContext 删除
func (r *wechatCache) DeleteContext(ctx context.Context, key string) error {
	return r.conn.Del(ctx, key).Err()
}
