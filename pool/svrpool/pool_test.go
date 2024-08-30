package svrpool

import (
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/dbutil/redispool"
)

func TestSvrPool(t *testing.T) {
	rdsCli, err := redispool.InitRedisWithConfig(&redis.Options{})
	if err != nil {
		t.Skip(err)
		return
	}
	p := NewServicePoolWithRedis(nil, rdsCli, "test")
	p.WithSetting("group", "key", 0)
	pool := p.Pool()
	svr, err := pool.AcquireString()
	if err != nil {
		t.Error(err)
		return
	}
	defer pool.ReleaseString(svr)

	t.Log(svr)
}
