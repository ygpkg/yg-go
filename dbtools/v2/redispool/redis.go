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

func InitRedis(group, key string) error {
	cfg := &config.RedisConfig{}
	err := settings.GetYaml(group, key, cfg)
	if err != nil {
		logs.Errorf("[dbv2/redispool] load redis config failed: %s", err)
		return err
	}

	rds := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	err = rds.Ping(context.Background()).Err()
	if err != nil {
		logs.Errorf("[dbv2/redispool] ping redis failed: %s", err)
		return err
	}
	stdRedis = rds

	InitCache(rds)
	return nil
}

func InitRedisWithConfig(cfg *redis.Options) (*redis.Client, error) {
	rds := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	err := rds.Ping(context.Background()).Err()
	if err != nil {
		logs.Errorf("[dbv2/redispool] ping redis failed: %s", err)
		return nil, err
	}
	stdRedis = rds

	InitCache(rds)
	return rds, nil
}

func Redis() *redis.Client {
	if stdRedis == nil {
		panic(fmt.Errorf("redis is nil"))
	}
	return stdRedis
}

func GetRedis() (*redis.Client, error) {
	if stdRedis == nil {
		return nil, fmt.Errorf("redis is nil")
	}
	return stdRedis, nil
}
