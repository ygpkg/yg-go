package svrpool

import (
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/logs"
)

var std *PoolManager

// RegistryServicePool 注册服务
func RegistryServicePool(group, key string) {
	if std == nil {
		std = &PoolManager{}
	}
	std.RegistryServicePool(group, key)
}

// AcquireService 获取服务
func AcquireService(key string, ttl time.Duration, retryTimes int64) (string, error) {
	if std == nil {
		logs.Errorf("svrpool not init")
		return "", fmt.Errorf("svrpool not init")
	}
	return std.AcquireService(key, ttl, retryTimes)
}

// ReleaseService 释放服务
func ReleaseService(key string, value string) {
	if std == nil {
		logs.Errorf("svrpool not init")
		return
	}
	std.ReleaseService(key, value)
}
