package pay

import (
	"context"
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/pay/paytype"
	"gorm.io/gorm"
)

type Pay interface {
	Prepay(payment *paytype.Payment) (string, error)
	QueryByTradeNo(payment *paytype.Payment) (*QueryResp, error)
	CloseOrder(payment *paytype.Payment) error
}

type QueryResp struct {
	TransactionId string
	TradeState    string
	SuccessTime   *time.Time
}

// NewPay 初始化支付
func NewPay(pay_type paytype.PayType, trade_tpye string) (Pay, error) {
	switch pay_type {
	case paytype.PayTypeWechat:
		return NewWxPay(trade_tpye)
	case paytype.PayTypeCash:
		return nil, fmt.Errorf("pay_type %s not support", pay_type)
	default:
		return nil, fmt.Errorf("pay_type %s not support", pay_type)
	}
}

// PlaceOrder 下订单 创建订单数据返回订单号
func PlaceOrder(db *gorm.DB, order *paytype.PayOrder, business int) (string, error) {
	// 生成订单号
	orderNo, err := NewOrderNo(context.Background(), business)
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

// InitiatePayment 对一个订单发起支付发起支付
func InitiatePayment(db *gorm.DB, order *paytype.PayOrder, pay_type paytype.PayType, trade_tpye string, expire_time *time.Time) (*paytype.Payment, string, error) {
	pay, err := NewPay(pay_type, trade_tpye)
	if err != nil {
		logs.Errorf("NewPay failed,err=%v", err)
		return nil, "", err
	}
	// 判断是否有正在支付的记录
	payments, err := paytype.GetPayPaymentByOrderNo(db, order.OrderNo, paytype.PayStatusPending)
	if err != nil {
		logs.Errorf("GetPayPaymentByOrderNo failed,err=%v", err)
		return nil, "", err
	}
	if len(payments) > 0 {
		// 有正在支付的记录，关闭他重新开启
		for _, payment := range payments {
			err = pay.CloseOrder(payment)
			if err != nil {
				logs.Errorf("CloseOrder failed,err=%v", err)
				return nil, "", err
			}
		}
	}

	// 生成支付号。
	tradeNo, err := NewTradeNo(context.Background(), order.OrderNo)
	if err != nil {
		logs.Errorf("NewTradeNo failed,err=%v", err)
		return nil, "", err
	}
	now := time.Now()
	// 创建支付表数据。
	payment := &paytype.Payment{
		Uin:         order.Uin,
		CompanyID:   order.CompanyID,
		TradeNo:     tradeNo,
		OrderNo:     order.OrderNo,
		Amount:      order.ShouldAmount,
		Description: order.Description,
		PayStatus:   paytype.PayStatusPending,
		PayType:     pay_type,
		TradeTpye:   trade_tpye,
		PayTime:     &now,
		ExpireTime:  expire_time,
	}

	// 发起预支付
	key, err := pay.Prepay(payment)
	if err != nil {
		logs.Errorf("Prepay failed,err=%v", err)
		return nil, "", err
	}
	payment.PrePaySign = key
	// 创建记录
	err = paytype.CreatePayPayment(db, payment)
	if err != nil {
		logs.Errorf("CreatePayPayment failed,err=%v", err)
		return nil, "", err
	}
	// 返回支付对象和预支付key
	return payment, key, nil
}

// QueryByTradeNo 根据订单号查询订单
func QueryByTradeNo(db *gorm.DB, payment *paytype.Payment) (string, error) {
	// 查询订单
	pay, err := NewPay(payment.PayType, payment.TradeTpye)
	if err != nil {
		logs.Errorf("QueryByTradeNo NewPay failed,err=%v", err)
		return "", err
	}
	resp, err := pay.QueryByTradeNo(payment)
	if err != nil {
		logs.Errorf("QueryByTradeNo QueryByTradeNo failed,err=%v", err)
		return "", err
	}
	switch resp.TradeState {
	case "NOTPAY":
		return "NOTPAY", nil
	case "REFUND":
		return "REFUND", nil
	case "CLOSED":
		// 更新支付表信息
		payment.PayStatus = paytype.PayStatusCancel
		err = paytype.SavePayPayment(db, payment)
		if err != nil {
			logs.Errorf("QueryByTradeNo SavePayPayment failed,err=%v", err)
			return "", err
		}
		return "CLOSED", nil
	case "SUCCESS":
		payment.PayStatus = paytype.PayStatusSuccess
		payment.PaySuccessTime = resp.SuccessTime
		payment.TransactionID = resp.TransactionId
		order, err := paytype.GetPayOrderByOrderNo(db, payment.OrderNo)
		if err != nil {
			logs.Errorf("QueryByTradeNo GetPayOrderByOrderNo failed,err=%v", err)
			return "", err
		}
		// 成功后订单时代发货状态
		order.OrderStatus = paytype.OrderStatusPendingSend
		order.PayStatus = paytype.PayStatusSuccess

		// 事务操作三个表
		err = db.Transaction(func(tx *gorm.DB) error {
			// 新增流水
			err := paytype.CreatePayPayment(tx, payment)
			if err != nil {
				logs.Errorf("QueryByTradeNo CreatePayPayment failed,err=%v", err)
				return err
			}
			// 更新支付表信息
			err = paytype.SavePayPayment(tx, payment)
			if err != nil {
				logs.Errorf("QueryByTradeNo SavePayPayment failed,err=%v", err)
				return err
			}
			// 更新订单表信息
			err = paytype.SavePayOrder(tx, order)
			if err != nil {
				logs.Errorf("QueryByTradeNo SavePayPayment failed,err=%v", err)
				return err
			}
			return nil
		})
		if err != nil {
			logs.Errorf("QueryByTradeNo Transaction failed,err=%v", err)
			return "", err
		}
		return "SUCCESS", nil
	default:
		return "", fmt.Errorf("unknown result type")
	}
}

// CloseOrder 关闭订单
func CloseOrder(db *gorm.DB, payment *paytype.Payment) error {
	// 查询订单
	pay, err := NewPay(payment.PayType, payment.TradeTpye)
	if err != nil {
		logs.Errorf("CloseOrder NewPay failed,err=%v", err)
		return err
	}
	err = pay.CloseOrder(payment)
	if err != nil {
		logs.Errorf("CloseOrder CloseOrder failed,err=%v", err)
		return err
	}
	// 更新支付表信息
	payment.PayStatus = paytype.PayStatusCancel
	err = paytype.SavePayPayment(db, payment)
	if err != nil {
		logs.Errorf("CloseOrder SavePayPayment failed,err=%v", err)
		return err
	}
	return nil
}
