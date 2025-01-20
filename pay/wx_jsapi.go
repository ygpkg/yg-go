package pay

import (
	"fmt"

	"github.com/ygpkg/yg-go/pay/paytype"
)

var _ Pay = (*JsApi)(nil)

type JsApi struct {
	// ctx     context.Context
	// cfg     *config.WXPayConfig
	// payment *paytype.Payment
}

// NativePrepay 预支付获取二维码链接
func (na *JsApi) Prepay(payment *paytype.Payment) (string, error) {
	fmt.Println("jsapi prepay")

	return "", nil
}

// NativeQueryOrderByTransactionID 根据支付号查询订单
func (na *JsApi) QueryByTradeNo(payment *paytype.Payment) (*QueryResp, error) {
	fmt.Println("jsapi queryByTradeNo")
	return nil, nil
}

// CloseOrder 关闭订单
func (na *JsApi) CloseOrder(payment *paytype.Payment) error {
	fmt.Println("jsapi closeOrder")
	return nil
}
