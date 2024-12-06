package svrpool

import (
	"context"
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/lifecycle"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/pool"
	"github.com/ygpkg/yg-go/settings"
)

// PoolManager 服务池管理器
type PoolManager struct {
	sync.RWMutex
	ctx  context.Context
	svrs map[string]*ServicePool
}

// RegistryServicePool 注册服务
func (pm *PoolManager) RegistryServicePool(group, key string) {
	pm.Lock()
	defer pm.Unlock()
	if pm.svrs == nil {
		pm.ctx = lifecycle.Std().Context()
		pm.svrs = make(map[string]*ServicePool)
	}
	if _, ok := pm.svrs[key]; ok {
		logs.Errorw("RegistryServicePool error", "key", key, "err", "key already exists")
		return
	}

	pm.svrs[key] = NewServicePool(context.Background(), group, key)
	logs.Infof("RegistryServicePool %s success", key)
}

// AcquireService 获取服务
func (pm *PoolManager) AcquireService(key string, interval time.Duration, retryTimes int) (string, error) {
	var svr string
	err := lifecycle.Retry(interval, retryTimes, func() (retry bool, err error) {
		pm.RLock()
		defer pm.RUnlock()
		if pm.svrs == nil {
			return true, fmt.Errorf("svrpool not init")
		}
		sp, ok := pm.svrs[key]
		if ok {
			return true, fmt.Errorf("svr %s not registered", key)
		}
		svr, err = sp.pool.AcquireString()
		if err != nil {
			return true, err
		}
		return false, nil
	})
	return svr, err
}

// ReleaseService 释放服务
func (pm *PoolManager) ReleaseService(key string, value string) {
	pm.RLock()
	defer pm.RUnlock()
	if pm.svrs == nil {
		return
	}
	sp, ok := pm.svrs[key]
	if ok {
		sp.pool.ReleaseString(value)
	}
}

// ServicePool 服务池
type ServicePool struct {
	ctx  context.Context
	pool pool.Pool
	// settingGroup 配置信息组名
	settingGroup string
	// settingKey 服务名 同时用来取配置信息
	settingKey string
	//服务配置
	conf config.ServicePoolConfig
}

// NewServicePool 创建服务池
func NewServicePool(ctx context.Context, group, key string) *ServicePool {
	var conf config.ServicePoolConfig
	if err := settings.GetYaml(group, key, &conf); err != nil {
		logs.Errorw("NewServicePool error", "key", key, "err", err)
		return nil
	}
	rdsKey := svrPoolRdsKey(key)
	rdsPool := pool.NewRedisPool(ctx, redispool.Std(), rdsKey, conf)
	return &ServicePool{
		ctx:          ctx,
		pool:         rdsPool,
		settingKey:   key,
		settingGroup: group,
		conf:         conf,
	}
}

// refreshSetting 定时刷新服务配置
func (s *ServicePool) refreshSetting() {
	conf, err := loadSetting(s.settingGroup, s.settingKey)
	if err != nil {
		logs.Errorw("loadSetting error", "key", s.settingKey, "err", err)
		return
	}
	if reflect.DeepEqual(conf, s.conf) {
		// 相等直接返回
		return
	}
	err = s.pool.RefreshConfig(conf)
	if err != nil {
		logs.Errorw("refresh setting error", "key", s.settingKey, "err", err)
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

func svrPoolRdsKey(key string) string {
	return "svrpool:" + key
}
