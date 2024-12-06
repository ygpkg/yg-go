package svrpool

import (
	"context"
	"reflect"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/pool"
	"github.com/ygpkg/yg-go/settings"
)

// ServicePool 服务池
type ServicePool struct {
	ctx  context.Context
	pool pool.Pool
	// group 配置信息组名
	group string
	// 服务名 同时用来取配置信息
	key string
	//服务配置
	conf config.ServicePoolConfig
}

// NewServicePool 创建服务池
func NewServicePool(ctx context.Context, pool pool.Pool, group, key string, conf config.ServicePoolConfig) *ServicePool {
	return &ServicePool{
		ctx:   ctx,
		pool:  pool,
		key:   key,
		group: group,
		conf:  conf,
	}
}

// NewServicePoolWithRedis 创建服务池
func NewServicePoolWithRedis(ctx context.Context, rdscli *redis.Client, group, key string) *ServicePool {
	setting, err := loadSetting(group, key)
	if err != nil {
		panic(err)
	}
	pool := pool.NewRedisPool(ctx, rdscli, key, setting)
	sp := NewServicePool(ctx, pool, group, key, setting)
	go func() {
		for {
			time.Sleep(time.Minute * 2)
			sp.refreshSetting()
		}
	}()
	logs.Infof("loadSetting success")
	return sp
}

// refreshSetting 定时刷新服务配置
func (s *ServicePool) refreshSetting() {
	conf, err := loadSetting(s.group, s.key)
	if err != nil {
		logs.Warnw("loadSetting error", "key", s.key, "err", err)
	}
	if reflect.DeepEqual(conf, s.conf) {
		// 相等直接返回
		return
	}
	err = s.pool.RefreshConfigs(conf)
	if err != nil {
		logs.Warnw("refresh setting error", "key", s.key, "err", err)
	}
	logs.Infof("refresh residpool setting success")
}

// loadSetting 从配置中加载服务
func loadSetting(group, key string) (config.ServicePoolConfig, error) {
	conf := config.ServicePoolConfig{}
	err := settings.GetYaml(group, key, &conf)
	if err != nil {
		logs.Warnw("loadSetting error", "err", err)
		return conf, err
	}
	return conf, nil
}
