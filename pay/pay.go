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
	State         string
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
	payments, err := paytype.GetPayPayment(db, order.OrderNo, paytype.PayStatusPending)
	if err != nil {
		logs.Errorf("GetPayPayment failed,err=%v", err)
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
	switch resp.State {
	case "NOTPAY":
		if time.Now().After(*payment.ExpireTime) {
			// 超时关闭订单
			err = CloseOrder(db, payment)
			if err != nil {
				logs.Errorf("QueryByTradeNo CloseOrder failed,err=%v", err)
				return "", err
			}
			return "CLOSED", nil
		}
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
		order.PayType = payment.PayType
		// 事务操作三个表
		err = db.Transaction(func(tx *gorm.DB) error {
			// 新增流水
			err := paytype.CreatePayStatement(tx, &paytype.PayStatement{
				Uin:             order.Uin,
				CompanyID:       order.CompanyID,
				OrderNo:         order.OrderNo,
				TransactionType: paytype.TransactionTypeIn,
				SubjectNo:       payment.TradeNo,
				Amount:          order.ShouldAmount,
			})
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
				logs.Errorf("QueryByTradeNo SavePayOrder failed,err=%v", err)
				return err
			}
			return nil
		})
		if err != nil {
			logs.Errorf("QueryByTradeNo Transaction failed,err=%v", err)
			return "", err
		}
		// 删除支付号key
		err = DeleteTradeNoKey(context.Background(), order.OrderNo)
		if err != nil {
			logs.Errorf("QueryByTradeNo DeleteTradeNoKey failed,err=%v", err)
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

// Refund 退款
func Refund(db *gorm.DB, ctx context.Context, refund *paytype.PayRefund) error {
	// 获取订单信息
	order, err := paytype.GetPayOrderByOrderNo(db, refund.OrderNo)
	if err != nil {
		logs.Errorf("get pay order error: %v", err)
		return err
	}
	// 判断支付信息是否已完成
	if order.PayStatus != "success" {
		logs.Errorf("payment.TradeState is not success TradeNo:%s", order.OrderNo)
		return fmt.Errorf("payment.TradeState is not success TradeNo:%s", order.OrderNo)
	}
	// 判断退款金额是否小于等于总金额并且不等于0
	if refund.Amount > order.ShouldAmount || refund.Amount == 0 {
		logs.Errorf("refund amount is not valid TradeNo:%s", order.OrderNo)
		return fmt.Errorf("refund amount is not valid TradeNo:%s", order.OrderNo)
	}
	// 查找付款表获取付款号
	payment, err := paytype.GetPayPaymentByOrderNo(db, order.OrderNo)
	if err != nil {
		logs.Errorf("get pay payment error: %v", err)
		return err
	}
	// 生成退款号
	refundNo, err := NewRefundNo(context.Background(), order.OrderNo)
	if err != nil {
		logs.Errorf("NewTradeNo failed,err=%v", err)
		return err
	}
	now := time.Now()
	refund.RefundNo = refundNo
	refund.PayStatus = paytype.PayStatusPendingRefund
	refund.RefundTime = &now
	refund.PayType = payment.PayType
	// 修改订单状态 待退款
	order.PayStatus = paytype.PayStatusPendingRefund
	// 判断是什么方式退款
	switch payment.PayType {
	case paytype.PayTypeWechat:
		// 微信退款
		err := WxRefund(ctx, payment, refund)
		if err != nil {
			logs.Errorf("WxRefund failed,err=%v", err)
			return err
		}
	case paytype.PayTypeCash:
		// 现金退款

	default:
		refund.PayStatus = paytype.PayStatusCancel
		return nil
	}
	err = db.Transaction(func(tx *gorm.DB) error {
		// 更新订单
		err = paytype.SavePayOrder(tx, order)
		if err != nil {
			logs.Errorf("WxRefund SavePayOrder failed,err=%v", err)
			return err
		}
		// 生成退款单
		err = paytype.CreatePayRefund(tx, refund)
		if err != nil {
			logs.Errorf("WxRefund CreatePayRefund failed,err=%v", err)
			return err
		}
		return nil
	})
	if err != nil {
		logs.Errorf("WxRefund Transaction failed,err=%v", err)
		return err
	}
	return nil
}

// QueryRefund 查询退款
func QueryRefund(db *gorm.DB, ctx context.Context, refund *paytype.PayRefund) (string, error) {
	resp := &QueryResp{}
	var err error
	switch refund.PayType {
	case paytype.PayTypeWechat:
		// 微信退款
		resp, err = WxQueryRefund(ctx, refund)
		if err != nil {
			logs.Errorf("WxQueryRefund failed,err=%v", err)
			return "", err
		}
	case paytype.PayTypeCash:
		// 现金退款
	default:
		return "", fmt.Errorf("paytype %s not support", refund.PayType)
	}

	switch resp.State {
	case "CLOSED":
		// 退款关闭
		return "CLOSED", nil
	case "ABNORMAL":
		// 退款异常
		return "ABNORMAL", fmt.Errorf("refund state is ABNORMAL")
	case "PROCESSING":
		return "PROCESSING", nil
	case "SUCCESS":
		refund.PayStatus = paytype.PayStatusSuccessRefund
		refund.RefundSuccessTime = resp.SuccessTime
		order, err := paytype.GetPayOrderByOrderNo(db, refund.OrderNo)
		if err != nil {
			logs.Errorf("QueryRefund GetPayOrderByOrderNo failed,err=%v", err)
			return "", err
		}
		order.PayStatus = paytype.PayStatusSuccessRefund
		// 事务操作三个表
		err = db.Transaction(func(tx *gorm.DB) error {
			// 新增流水
			err := paytype.CreatePayStatement(tx, &paytype.PayStatement{
				Uin:             order.Uin,
				CompanyID:       order.CompanyID,
				OrderNo:         order.OrderNo,
				TransactionType: paytype.TransactionTypeOut,
				SubjectNo:       refund.RefundNo,
				Amount:          refund.Amount,
			})
			if err != nil {
				logs.Errorf("QueryRefund CreatePayPayment failed,err=%v", err)
				return err
			}
			// 更新退款表信息
			err = paytype.SavePayRefund(tx, refund)
			if err != nil {
				logs.Errorf("QueryByTradeNo SavePayRefund failed,err=%v", err)
				return err
			}
			// 更新订单表信息
			err = paytype.SavePayOrder(tx, order)
			if err != nil {
				logs.Errorf("QueryByTradeNo SavePayOrder failed,err=%v", err)
				return err
			}
			return nil
		})
		if err != nil {
			logs.Errorf("QueryRefund Transaction failed,err=%v", err)
			return "", err
		}
		// 删除支付号key
		err = DeleteRefundNoKey(context.Background(), order.OrderNo)
		if err != nil {
			logs.Errorf("QueryRefund DeleteRefundNoKey failed,err=%v", err)
			return "", err
		}
		return "SUCCESS", nil
	default:
		return "", fmt.Errorf("unknown result type")
	}
}
