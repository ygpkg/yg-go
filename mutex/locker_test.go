package mutex

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestRedisLockInfo(t *testing.T) {
	if true {
		return
	}
	rds := redis.NewClient(&redis.Options{
		Addr: "127.0.0.1:6379",
	})
	ctx := context.Background()
	if err := rds.Ping(ctx).Err(); err != nil {
		t.Skip(err)
		return
	}
	lkr := NewRedisLocker(rds, "test", time.Second*10)
	go func() {
		for i := 0; i < 5; i++ {
			lkr.Lock()
			t.Logf("A: Locked")
			time.Sleep(time.Second * 2)
			lkr.Unlock()
			t.Logf("A: Unlocked")
			time.Sleep(time.Second / 10)
		}
	}()

	for i := 0; i < 5; i++ {
		lkr.Lock()
		t.Logf("B: Locked")
		time.Sleep(time.Second * 1)
		lkr.Unlock()
		t.Logf("B: Unlocked")
		time.Sleep(time.Second / 10)
	}
}
