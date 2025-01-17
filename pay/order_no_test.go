package pay

import (
	"context"
	"fmt"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/dbtools/redispool"
)

func TestNewOrderNo(t *testing.T) {
	redispool.InitRedisWithConfig(&redis.Options{
		Addr:     "192.168.1.106:6379",
		Password: "skf021120",
		DB:       0,
	})
	for i := 0; i < 100; i++ {
		// 生成100个订单号
		no, err := NewOrderNo(context.Background(), 1)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Println("OrderNo", no)
	}
}

func TestNewTradeNo(t *testing.T) {
	redispool.InitRedisWithConfig(&redis.Options{
		Addr:     "192.168.1.106:6379",
		Password: "skf021120",
		DB:       0,
	})
	no, err := NewOrderNo(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OrderNo", no)
	tno, err := NewTradeNo(context.Background(), no)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("TradeNo", tno)
}

func TestNewRefundNo(t *testing.T) {
	redispool.InitRedisWithConfig(&redis.Options{
		Addr:     "192.168.1.106:6379",
		Password: "skf021120",
		DB:       0,
	})
	no, err := NewOrderNo(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("OrderNo", no)
	rno, err := NewRefundNo(context.Background(), no)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("RefundNo", rno)
}
