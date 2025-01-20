package pay

import (
	"fmt"

	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/ygpkg/yg-go/pay/paytype"
)

type Pay interface {
	Prepay() (string, error)
	QueryByTradeNo() (*payments.Transaction, error)
	CloseOrder() error
}

// NewPay 初始化支付
func NewPay(payment *paytype.Payment, pay_type paytype.PayType, trade_tpye string) (Pay, error) {
	switch pay_type {
	case paytype.PayTypeWechat:
		return NewWxPay(payment, trade_tpye)
	case paytype.PayTypeCash:
		return nil, fmt.Errorf("pay_type %s not support", pay_type)
	default:
		return nil, fmt.Errorf("pay_type %s not support", pay_type)
	}
}
