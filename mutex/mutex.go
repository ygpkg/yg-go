package mutex

import (
	"context"

	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/lifecycle"
)

var std *ClusterMutex

func IsMaster() bool {
	if std == nil {
		return false
	}
	return std.IsMaster()
}

func InitCluster(ctx context.Context) {
	if std == nil {
		std = NewClusterMutex(lifecycle.Std().Context(), redispool.Std(), "default_cluster_mutex")
	}
}
