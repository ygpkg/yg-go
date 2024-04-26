package redispool

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/settings"
	"github.com/ygpkg/yg-go/utils/logs"
)

var stdRedis *redis.Client

func InitRedis(group, key string) error {
	cfg := &redis.Options{}
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

func Redis() *redis.Client {
	if stdRedis == nil {
		panic(fmt.Errorf("redis is nil"))
	}
	return stdRedis
}

// WechatCache 兼容微信包的cache
func WechatCache() *wechatCache {
	return &wechatCache{
		ctx:  context.Background(),
		conn: Redis(),
	}
}
