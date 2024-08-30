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

}
