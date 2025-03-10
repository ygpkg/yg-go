package main

import (
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/lifecycle"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/mutex"
)

func main() {
	redispool.InitRedisWithConfig(&redis.Options{Addr: "127.0.0.1:6379"})
	go func() {
		for i := 0; i < 10; i++ {
			logs.Infof("is master: %v", mutex.IsMaster())
			time.Sleep(time.Second * 9)
		}
	}()
	lifecycle.Std().WaitExit()
}
