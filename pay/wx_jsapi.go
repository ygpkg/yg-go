package pay

import (
	"fmt"

	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
)

var _ Pay = (*JsApi)(nil)

type JsApi struct {
	// ctx     context.Context
	// cfg     *config.WXPayConfig
	// payment *paytype.Payment
}

// NativePrepay 预支付获取二维码链接
func (na *JsApi) Prepay() (string, error) {
	fmt.Println("jsapi prepay")

	return "", nil
}

// NativeQueryOrderByTransactionID 根据支付号查询订单
func (na *JsApi) QueryByTradeNo() (*payments.Transaction, error) {
	fmt.Println("jsapi queryByTradeNo")
	return nil, nil
}

// CloseOrder 关闭订单
func (na *JsApi) CloseOrder() error {
	fmt.Println("jsapi closeOrder")
	return nil
}
