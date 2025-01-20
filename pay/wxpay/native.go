package wxpay

import (
	"context"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/pay/paytype"
)

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

// NativePrepay 预支付获取二维码链接
func NativePrepay(cfg *config.WXPayConfig, payment *paytype.Payment, description string) (string, error) {
	ctx := context.Background()
	client, err := InitWxPay(ctx, cfg)
	if err != nil {
		logs.Errorf("init wechat pay client err:%v", err)
		return "", err
	}
	svc := native.NativeApiService{Client: client}
	resp, _, err := svc.Prepay(ctx,
		native.PrepayRequest{
			Appid:       core.String(cfg.AppID),        // appid
			Mchid:       core.String(cfg.MchID),        // 商户号
			Description: core.String(description),      // 商品描述
			OutTradeNo:  core.String(payment.TradeNo),  // 自定义订单编号
			TimeExpire:  core.Time(payment.ExpireTime), // 交易结束时间
			NotifyUrl:   core.String(cfg.NotifyURL),    // 回调地址
			Amount: &native.Amount{
				Currency: core.String("CNY"),                            // CNY：人民币，境内商户号仅支持人民币。
				Total:    core.Int64(int64(payment.Amount.Val() * 100)), // 单位为分 1元应填写100分
			},
		},
	)
	if err != nil {
		// 处理错误
		logs.Errorf("call Prepay err:%s", err)
		return "", err
	}
	return *resp.CodeUrl, nil
}

// NativeQueryOrderByTransactionID 根据支付号查询订单
func NativeQueryOrderByOutTradeNo(cfg *config.WXPayConfig, payment *paytype.Payment) (*payments.Transaction, error) {
	ctx := context.Background()
	client, err := InitWxPay(ctx, cfg)
	if err != nil {
		logs.Errorf("init wechat pay client err:%v", err)
		return nil, err
	}
	svc := native.NativeApiService{Client: client}
	resp, result, err := svc.QueryOrderByOutTradeNo(ctx,
		native.QueryOrderByOutTradeNoRequest{
			OutTradeNo: core.String(payment.TradeNo),
			Mchid:      core.String(cfg.MchID),
		},
	)
	if err != nil {
		// 处理错误
		logs.Errorf("call QueryOrderByOutTradeNo err:%v", err)
		return nil, err
	}
	if result.Response.StatusCode != 200 {
		logs.Errorf("call QueryOrderByOutTradeNo err:%s", result.Response.Status)
		return nil, err
	}
	return resp, nil
}

// NativeCloseOrder 关闭订单
func NativeCloseOrder(cfg *config.WXPayConfig, payment *paytype.Payment) error{
	ctx := context.Background()
	client, err := InitWxPay(ctx, cfg)
	if err != nil {
		logs.Errorf("init wechat pay client err:%v", err)
		return  err
	}
	svc := native.NativeApiService{Client: client}
	result, err := svc.CloseOrder(ctx,
		native.CloseOrderRequest{
			OutTradeNo: core.String(payment.TradeNo),
			Mchid:      core.String(cfg.MchID),
		},
	)
	if err != nil {
		// 处理错误
		logs.Errorf("call QueryOrderByOutTradeNo err:%v", err)
		return err
	}
	if result.Response.StatusCode != 204 {
		logs.Errorf("call QueryOrderByOutTradeNo err:%v", result.Response.Status)
		return  err
	}
	return nil
}
