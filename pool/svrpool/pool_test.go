package svrpool

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/logs"
)

func TestSvrPool(t *testing.T) {
	_, err := redispool.InitRedisWithConfig(&redis.Options{
		Addr:     "127.0.0.1:6379",
		Password: "skf021120",
		DB:       1,
	})
	if err != nil {
		t.Skip(err)
		return
	}
	db()

	rp := NewServicePool(context.Background(), "core", "test:now")
	var v map[string]interface{}
	rp.pool.AcquireDecode(&v)
	fmt.Println("111111111111111111111111", v)
	time.Sleep(time.Minute * 3)
	rp.pool.ReleaseEncode(v)
	time.Sleep(time.Minute * 10)
}

func db() {
	cfg, err := config.LoadCoreConfigFromFile("D:\\GoProject\\src\\roc\\apps\\llm\\conf\\test\\config.yaml")
	if err != nil {
		fmt.Println(err)
		return
	}
	err = dbtools.InitMutilMySQL(cfg.MainConf.MysqlConns)
	if err != nil {
		logs.Errorf("[main] connect mysql failed, %s", err)
		return
	}
	db := dbtools.Core()
	db.Logger = logs.GetGorm("gorm")

}
