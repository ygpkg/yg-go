package pool

import (
	"github.com/ygpkg/yg-go/config"
)

// ResourceID 资源ID
type ResourceID = string

// Pool 资源池接口
type Pool interface {
	// AcquireString 从资源池中获取一个资源
	AcquireString() (ResourceID, string, error)
	// ReleaseString 释放一个资源到资源池
	ReleaseString(ResourceID) error
	// Clear 清空资源池
	Clear()
	// RefreshConfigs 刷新配置
	RefreshConfig(config.ServicePoolConfig) error
}
