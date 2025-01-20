package wxpay

import (
	"context"
	"fmt"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/pay/paytype"
	"github.com/ygpkg/yg-go/settings"
)

type WxPay interface {
	Prepay() (string, error)
	QueryByTradeNo() (*payments.Transaction, error)
	CloseOrder() error
}

// InitWxPay 初始化微信支付
func InitWxPay(ctx context.Context, cfg *config.WXPayConfig) (*core.Client, error) {
	// 加载私钥生成签名
	mchPrivateKey, err := utils.LoadPrivateKey(cfg.Pemkey)
	if err != nil {
		logs.Errorf("load merchant private key error %v", err)
		return nil, err
	}

	// 使用商户私钥等初始化 client，并使它具有自动定时获取微信支付平台证书的能力
	opts := []core.ClientOption{
		option.WithWechatPayAutoAuthCipher(cfg.MchID, cfg.MchCertificateSerialNumber, mchPrivateKey, cfg.MchAPIv3Key),
	}

	client, err := core.NewClient(ctx, opts...)
	if err != nil {
		logs.Errorf("new wechat pay client err:%v", err)
		return nil, err
	}

	return client, nil
}

// NewWxPay 初始化微信支付
func NewWxPay(payment *paytype.Payment, pay_type string) (WxPay, error) {
	var (
		ctx = context.Background()
		cfg = &config.WXPayConfig{}
		key = "wxpay"
	)
	err := settings.GetYaml(settings.SettingGroupCore, key, &cfg)
	if err != nil {
		logs.Errorf("get wxpay config error: %v", err)
		return nil, err
	}
	client, err := InitWxPay(ctx, cfg)
	if err != nil {
		logs.Errorf("init wechat pay client err:%v", err)
		return nil, err
	}
	switch pay_type {
	case "native":
		return &Native{
			ctx:     ctx,
			cfg:     cfg,
			payment: payment,
			client:  client,
		}, nil
	case "jsapi":
		return &JsApi{}, nil
	default:
		return nil, fmt.Errorf("pay_type %s not support", pay_type)
	}

}
