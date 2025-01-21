package pay

import (
	"context"
	"fmt"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/pay/paytype"
	"github.com/ygpkg/yg-go/settings"
)

// initWxPay 初始化微信支付
func initWxPay(ctx context.Context, cfg *config.WXPayConfig) (*core.Client, error) {
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
func NewWxPay(pay_type string) (Pay, error) {
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
	client, err := initWxPay(ctx, cfg)
	if err != nil {
		logs.Errorf("init wechat pay client err:%v", err)
		return nil, err
	}
	switch pay_type {
	case "native":
		return &Native{
			ctx:    ctx,
			cfg:    cfg,
			client: client,
		}, nil
	case "jsapi":
		return &JsApi{}, nil
	default:
		return nil, fmt.Errorf("pay_type %s not support", pay_type)
	}

}

// WxRefund 微信支付退款
func WxRefund(ctx context.Context, payment *paytype.Payment, refund *paytype.PayRefund) error {
	var (
		cfg = &config.WXPayConfig{}
		key = "wxpay"
	)
	err := settings.GetYaml(settings.SettingGroupCore, key, &cfg)
	if err != nil {
		logs.Errorf("get wxpay config error: %v", err)
		return err
	}
	client, err := initWxPay(ctx, cfg)
	if err != nil {
		logs.Errorf("init wechat pay client err:%v", err)
		return err
	}
	svc := refunddomestic.RefundsApiService{Client: client}
	req := refunddomestic.CreateRequest{
		OutTradeNo:  core.String(payment.TradeNo),
		OutRefundNo: core.String(refund.RefundNo),
		Reason:      core.String(refund.Reason),
		NotifyUrl:   core.String(cfg.NotifyURL),
		Amount: &refunddomestic.AmountReq{
			Currency: core.String("CNY"),
			Refund:   core.Int64(int64(refund.Amount.Val() * 100)),
			Total:    core.Int64(int64(payment.Amount.Val() * 100)),
		},
	}
	refund.RefundReq, err = paytype.JsonString(req)
	if err != nil {
		logs.Errorf("json marshal error: %v", err)
		return err
	}
	resp, result, err := svc.Create(ctx, req)
	if err != nil {
		logs.Errorf("call Prepay err:%s", err)
		return err
	}
	if result.Response.StatusCode != 200 {
		logs.Errorf("call Prepay err:%s", result.Response.Body)
		return fmt.Errorf("call Prepay err:%s", result.Response.Body)
	}
	if *resp.Status == "ABNORMAL" {
		logs.Errorf("call Prepay err:%s", result.Response.Body)
		return fmt.Errorf("call Prepay err:%s", result.Response.Body)
	}
	return nil
}

// QueryRefund 查询退款
func WxQueryRefund(ctx context.Context, refund *paytype.PayRefund) (*QueryResp, error) {
	var (
		cfg = &config.WXPayConfig{}
		key = "wxpay"
	)
	err := settings.GetYaml(settings.SettingGroupCore, key, &cfg)
	if err != nil {
		logs.Errorf("get wxpay config error: %v", err)
		return nil, err
	}
	client, err := initWxPay(ctx, cfg)
	if err != nil {
		logs.Errorf("init wechat pay client err:%v", err)
		return nil, err
	}
	svc := refunddomestic.RefundsApiService{Client: client}
	resp, result, err := svc.QueryByOutRefundNo(ctx,
		refunddomestic.QueryByOutRefundNoRequest{
			OutRefundNo: core.String(refund.RefundNo),
		},
	)
	if err != nil {
		// 处理错误
		logs.Errorf("call QueryByOutRefundNo err:%v", err)
		return nil, err
	}
	if result.Response.StatusCode != 200 {
		logs.Errorf("call QueryOrderByOutTradeNo err:%s", result.Response.Status)
		return nil, err
	}
	queryResp := &QueryResp{
		State: string(*resp.Status),
	}
	if queryResp.State == "SUCCESS" {
		refund.RefundResp, err = paytype.JsonString(resp)
		if err != nil {
			logs.Errorf("call QueryOrderByOutTradeNo err:%s", err)
			return nil, err
		}
		queryResp.SuccessTime = resp.SuccessTime
		queryResp.TransactionId = *resp.TransactionId
	}
	return queryResp, nil
}
