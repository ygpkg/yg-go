package mutex

import (
	"os"

	"github.com/google/uuid"
	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/lifecycle"
)

var std *ClusterMutex

// Option 定义选项函数类型
type Option func(*config)

// config 配置
type config struct {
	mutexKey string
}

// WithMutexKey 设置 mutex key
func WithMutexKey(key string) Option {
	return func(c *config) {
		c.mutexKey = key
	}
}

// IsMaster 判断是否为主节点
func IsMaster(options ...Option) bool {
	// 默认配置
	cfg := &config{
		mutexKey: "default_cluster_mutex",
	}

	// 应用选项
	for _, opt := range options {
		opt(cfg)
	}

	if std == nil {
		nodeID := os.Getenv("HOSTNAME")
		if nodeID == "" {
			nodeID = uuid.NewString()
		}
		std = NewClusterMutex(
			lifecycle.Std().Context(),
			redispool.Std(),
			cfg.mutexKey,
			nodeID,
		)
	}
	return std.IsMaster()
}
