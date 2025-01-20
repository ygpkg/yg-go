package wxpay

import (
	"context"
	"time"

	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/pay"
	"github.com/ygpkg/yg-go/pay/paytype"
	"gorm.io/gorm"
)

// PlaceOrder 下订单
func PlaceOrder(db *gorm.DB, order *paytype.PayOrder, business int) (string, error) {
	// 生成订单号
	orderNo, err := pay.NewOrderNo(context.Background(), business)
	if err != nil {
		logs.Errorf("NewOrderNo failed,err=%v", err)
		return "", err
	}
	order.OrderNo = orderNo
	order.PayStatus = paytype.PayStatusPending
	order.OrderStatus = paytype.OrderStatusPendingPay
	// 创建订单
	err = db.Create(order).Error
	if err != nil {
		logs.Errorf("Create order failed,err=%v", err)
		return "", err
	}
	// 返回订单号
	return orderNo, nil
}

// InitiatePayment 发起支付
func InitiatePayment(db *gorm.DB, order *paytype.PayOrder, trade_tpye string, expire_time time.Time) (WxPay, string, error) {
	// 生成支付号。
	tradeNo, err := pay.NewTradeNo(context.Background(), order.OrderNo)
	if err != nil {
		logs.Errorf("NewTradeNo failed,err=%v", err)
		return nil, "", err
	}
	// 创建支付表数据。
	payment := &paytype.Payment{
		Uin:         order.Uin,
		CompanyID:   order.CompanyID,
		TradeNo:     tradeNo,
		OrderNo:     order.OrderNo,
		Amount:      order.ShouldAmount,
		Description: order.Description,
		PayStatus:   paytype.PayStatusPending,
		PayType:     paytype.PayTypeWechat,
		TradeTpye:   trade_tpye,
		PayTime:     time.Now(),
		ExpireTime:  expire_time,
	}
	// 发起预支付
	wx, err := NewWxPay(payment, trade_tpye)
	if err != nil {
		logs.Errorf("NewWxPay failed,err=%v", err)
		return nil, "", err
	}
	// 返回预支付key
	key, err := wx.Prepay()
	if err != nil {
		logs.Errorf("Prepay failed,err=%v", err)
		return nil, "", err
	}
	payment.PrePaySign = key
	return wx, key, nil
}
