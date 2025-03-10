package mutex

import (
	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/lifecycle"
)

var std *ClusterMutex

func IsMaster() bool {
	if std == nil {
		std = NewClusterMutex(lifecycle.Std().Context(), redispool.Std(), "default_cluster_mutex")
	}
	return std.IsMaster()
}
