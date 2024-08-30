package svrpool

import (
	"context"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/pool"
	"github.com/ygpkg/yg-go/settings"
)

// ServicePool 服务池
type ServicePool struct {
	sync.RWMutex
	ctx     context.Context
	pa, pb  pool.Pool
	current string

	skey, sgroup string

	autoReloadOnce sync.Once
}

// NewServicePool 创建服务池
func NewServicePool(ctx context.Context, pa, pb pool.Pool) *ServicePool {
	return &ServicePool{
		ctx:     ctx,
		pa:      pa,
		pb:      pb,
		current: "A",
	}
}

// NewServicePoolWithRedis 创建服务池
func NewServicePoolWithRedis(ctx context.Context, rdscli *redis.Client, key string) *ServicePool {
	pa := pool.NewRedisPool(ctx, rdscli, key+".a")
	pb := pool.NewRedisPool(ctx, rdscli, key+".b")
	return NewServicePool(ctx, pa, pb)
}

// WithSetting 从配置中加载服务
func (p *ServicePool) WithSetting(group, key string, autoReload time.Duration) error {
	p.skey = key
	p.sgroup = group
	if p.current == "A" {
		return loadSetting(p.pa, group, key)
	}
	if autoReload == 0 {
		return nil
	}
	p.autoReloadOnce.Do(func() {
		go p.reloadSettingRoutine(autoReload)
	})
	return nil
}

// Pool 获取当前服务池
func (p *ServicePool) Pool() pool.Pool {
	p.RLock()
	defer p.RUnlock()
	if p.current == "A" {
		return p.pa
	}
	return p.pb
}

// Switch 切换服务池
func (p *ServicePool) Switch() {
	p.Lock()
	defer p.Unlock()
	if p.current == "A" {
		if p.pb == nil {
			logs.ErrorContextf(p.ctx, "ServicePool Switch failed, pb is nil")
			return
		}
		p.current = "B"
		return
	}
	p.current = "A"
}

// Switch 切换服务池
func (p *ServicePool) reloadSettingRoutine(dur time.Duration) {
	ticker := time.NewTicker(dur)

	for {
		select {
		case <-ticker.C:
			if p.current == "A" {
				err := loadSetting(p.pb, p.sgroup, p.skey)
				if err != nil {
					logs.Errorf("ServicePool reloadSettingRoutine loadSetting failed,err=%v", err)
				}
			} else {
				err := loadSetting(p.pa, p.sgroup, p.skey)
				if err != nil {
					logs.Errorf("ServicePool reloadSettingRoutine loadSetting failed,err=%v", err)
				}
			}
			p.Switch()
		case <-p.ctx.Done():
			ticker.Stop()
			logs.Infof("ServicePool reloadSettingRoutine exit")
			return
		}
	}
}

// loadSetting 从配置中加载服务
func loadSetting(pool pool.Pool, group, key string) error {
	servers := config.ServicePoolConfig{}
	err := settings.GetYaml(group, key, &servers)
	if err != nil {
		return err
	}
	pool.Clear()
	for _, svrItem := range servers.Services {
		for i := 0; i < svrItem.Cap; i++ {
			pool.Release(svrItem.Name)
		}
	}
	return nil
}

// loadServiceList 加载服务
func loadServiceList(pool pool.Pool, servers *config.ServicePoolConfig) error {
	pool.Clear()
	for _, svrItem := range servers.Services {
		for i := 0; i < svrItem.Cap; i++ {
			pool.Release(svrItem.Name)
		}
	}
	return nil
}
