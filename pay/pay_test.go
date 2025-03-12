package pay

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/pay/paytype"
)

var (
	group = "core"
	key   = "wxpay"
)

func TestPlaceOrder(t *testing.T) {
	// go test -run TestPlaceOrder
	dbtools.InitMutilMySQL(map[string]string{
		"default": "",
		"core":    "",
	})
	redispool.InitRedisWithConfig(&redis.Options{
		Addr:     "",
		Password: "",
		DB:       0,
	})
	paytype.InitDB(dbtools.Std())
	orderNo, err := PlaceOrder(dbtools.Std(), &paytype.PayOrder{
		Uin:           1,
		CompanyID:     0,
		Description:   "vip服务",
		TotalAmount:   0.01, //
		ShouldAmount:  0.01,
		ExpireTime:    nil,
		OrderSnapshot: "快照快照快照",
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(orderNo)
}

func TestInitiatePayment(t *testing.T) {
	// go test -run TestInitiatePayment
	dbtools.InitMutilMySQL(map[string]string{
		"default": "",
		"core":    "",
	})
	redispool.InitRedisWithConfig(&redis.Options{
		Addr:     "",
		Password: "",
		DB:       0,
	})
	expire := time.Now().Add(5 * time.Minute)
	var order paytype.PayOrder
	err := dbtools.Std().Where("id = ?", 9).First(&order).Error
	if err != nil {
		t.Fatal(err)
	}
	payment, key, err := InitiatePayment(dbtools.Std(),
		&order, paytype.PayTypeWechat,
		"native", group, key, &expire)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(key)
	fmt.Println(payment)
}

func TestQueryByTradeNo(t *testing.T) {
	// go test -run TestQueryByTradeNo
	dbtools.InitMutilMySQL(map[string]string{
		"default": "",
		"core":    "",
	})
	redispool.InitRedisWithConfig(&redis.Options{
		Addr:     "",
		Password: "",
		DB:       0,
	})
	var payment paytype.Payment
	err := dbtools.Std().Where("id = ?", 9).First(&payment).Error
	if err != nil {
		t.Fatal(err)
	}
	str, err := QueryByTradeNo(dbtools.Std(), &payment, group, key)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(str)
}

func TestCloseOrder(t *testing.T) {
	// go test -run TestCloseOrder
	dbtools.InitMutilMySQL(map[string]string{
		"default": "",
		"core":    "",
	})
	redispool.InitRedisWithConfig(&redis.Options{
		Addr:     "",
		Password: "",
		DB:       0,
	})
	var payment paytype.Payment
	err := dbtools.Std().Where("id = ?", 2).First(&payment).Error
	if err != nil {
		t.Fatal(err)
	}
	err = CloseOrder(dbtools.Std(), &payment, group, key)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRefund(t *testing.T) {
	// go test -run TestRefund
	dbtools.InitMutilMySQL(map[string]string{
		"default": "",
		"core":    "",
	})
	redispool.InitRedisWithConfig(&redis.Options{
		Addr:     "",
		Password: "",
		DB:       0,
	})
	var order paytype.PayOrder
	err := dbtools.Std().Where("id = ?", 9).First(&order).Error
	if err != nil {
		t.Fatal(err)
	}
	err = Refund(dbtools.Std(), context.Background(), &paytype.PayRefund{
		Uin:       order.Uin,
		CompanyID: order.CompanyID,
		OrderNo:   order.OrderNo,
		Reason:    "测试退款",
		Amount:    0.01,
	}, "core", "wxpay")
	if err != nil {
		t.Fatal(err)
	}
}

func TestQueryRefund(t *testing.T) {
	// go test -run TestQueryRefund
	dbtools.InitMutilMySQL(map[string]string{
		"default": "",
		"core":    "",
	})
	redispool.InitRedisWithConfig(&redis.Options{
		Addr:     "192.168.1.106:6379",
		Password: "",
		DB:       0,
	})
	var refund paytype.PayRefund
	err := dbtools.Std().Where("id = ?", 1).First(&refund).Error
	if err != nil {
		t.Fatal(err)
	}
	str, err := QueryRefund(dbtools.Std(), context.Background(), &refund, group, key)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println(str)
}
