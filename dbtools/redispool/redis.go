package redispool

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
)

var stdRedis *redis.Client

// InitRedis 初始化redis连接
func InitRedis(group, key string) error {
	cfg := &config.RedisConfig{}
	err := settings.GetYaml(group, key, cfg)
	if err != nil {
		logs.Errorf("[dbutil] load redis config failed, %s", err)
		return err
	}

	rds := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password, // no password set
		DB:       cfg.DB,       // use default DB
	})

	err = rds.Ping(context.Background()).Err()
	if err != nil {
		logs.Errorf("[dbutil] ping redis failed, %s", err)
		return err
	}
	stdRedis = rds

	InitCache(rds)
	return nil
}

// InitRedisWithConfig 初始化redis连接
func InitRedisWithConfig(cfg *redis.Options) (*redis.Client, error) {
	rds := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password, // no password set
		DB:       cfg.DB,       // use default DB
	})

	err := rds.Ping(context.Background()).Err()
	if err != nil {
		logs.Errorf("[dbutil] ping redis failed, %s", err)
		return nil, err
	}
	stdRedis = rds

	InitCache(rds)
	return rds, nil
}

// Redis 获取redis连接
func Redis() *redis.Client {
	if stdRedis == nil {
		panic(fmt.Errorf("redis is nil"))
	}
	return stdRedis
}

// GetRedis 获取redis连接, 可能为nil
func GetRedis() (*redis.Client, error) {
	if stdRedis == nil {
		return nil, fmt.Errorf("redis is nil")
	}
	return stdRedis, nil
}
