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
	servers := config.ServicePoolConfig{}
	servers.Services = append(servers.Services, a, b)
	duration := 1 * time.Hour
	rsh := NewRedisHashPool(context.Background(), client, duration, "knownow", "know", servers)
	aa, _ := rsh.AcquireKeyIndex("deepseek.com")
	bb, _ := rsh.AcquireKeyIndex("chatgpt.com")
	fmt.Println(aa, bb)
	time.Sleep(2 * time.Minute)
	rsh.ReleaseKeyIndex("1_deepseek.com")
}
