package pay

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/logs"
)

// rediskey 前缀
const (
	OrderKeyPrefix  = "pay:order_no:%s"
	TradeKeyPrefix  = "pay:trade_no:%s"
	RefundKeyPrefix = "pay:refund_no:%s"
)

const (
	TradeNoPrefix  = "pay%s"
	RefundNoPrefix = "re%s"
)

// NewOrderNo 生成订单号 business 业务类型 return 订单号
func NewOrderNo(ctx context.Context, business int) (string, error) {
	now := time.Now() // 获取当前时间
	YYMMDDHHmm := now.Format("06010215")
	// 生成订单号前缀
	orderNoPrefix := strconv.Itoa(business) + YYMMDDHHmm
	// 生成 Redis 键
	redisKeyPrefix := fmt.Sprintf(OrderKeyPrefix, orderNoPrefix)
	// 使用 Redis 的自增操作
	increment, err := redispool.Std().Incr(ctx, redisKeyPrefix).Result()
	if err != nil {
		logs.Errorf("redispool call Incr failed,err=%v", err)
		return "", err
	}
	// 如果是每秒的第一个订单号，设置键的过期时间
	// 一小时一个key，存12小时
	if increment == 1 {
		redispool.Std().Expire(ctx, redisKeyPrefix, time.Hour*12)
	}
	// 生成 0 到 99 的随机数
	randomNum := rand.Intn(100)
	randomNumStr := fmt.Sprintf("%02d", randomNum)
	// 生成订单号：前缀 + 两位随机数 + 4 位自增值
	orderNo := orderNoPrefix + randomNumStr + fmt.Sprintf("%04d", increment)
	return orderNo, nil
}

// NewTradeNo 生成支付号
func NewTradeNo(ctx context.Context, order_no string) (string, error) {
	// 拼接前缀
	redisKey := fmt.Sprintf(TradeKeyPrefix, order_no)
	// 使用 Redis 的自增操作
	increment, err := redispool.Std().Incr(ctx, redisKey).Result()
	if err != nil {
		logs.Errorf("redispool call Incr failed,err=%v", err)
		return "", err
	}
	// 订单结束删除key
	tradeno := fmt.Sprintf(TradeNoPrefix, order_no) + fmt.Sprintf("%04d", increment)
	return tradeno, nil
}

// NewRefundNo 生成退款号
func NewRefundNo(ctx context.Context, order_no string) (string, error) {
	// 拼接前缀
	redisKey := fmt.Sprintf(RefundKeyPrefix, order_no)
	// 使用 Redis 的自增操作
	increment, err := redispool.Std().Incr(ctx, redisKey).Result()
	if err != nil {
		logs.Errorf("redispool call Incr failed,err=%v", err)
		return "", err
	}
	// 订单结束删除key
	refundno := fmt.Sprintf(RefundNoPrefix, order_no) + fmt.Sprintf("%04d", increment)
	return refundno, nil
}
