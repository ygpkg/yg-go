package pool

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/config"
)

func TestHashPool(t *testing.T) {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       1,
	})
	a := config.ServiceInfo{
		Name: "deepseek.com",
		Cap:  3,
	}
	b := config.ServiceInfo{
		Name: "chatgpt.com",
		Cap:  2,
	}
	servers := config.ServicePoolConfig{Expire: 10 * time.Second}
	servers.Services = append(servers.Services, a, b)
	rsh := NewRedisPool(context.Background(), client, "knownow:konw", servers)

	f, err := rsh.Acquire()
	if err != nil {
		fmt.Println(err)
	}
	fmt.Println(f)
	bb, _ := rsh.Acquire()
	fmt.Println(bb)
	time.Sleep(1 * time.Minute)
	rsh.Release(f)
	aa, _ := rsh.Acquire()
	fmt.Println(aa)
}
