package pay

import (
	"context"
	"fmt"
	"time"

	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments/native"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/pay/paytype"
)

var _ Pay = (*Native)(nil)

type Native struct {
	ctx    context.Context
	cfg    *config.WXPayConfig
	client *core.Client
}

// NativePrepay 预支付获取二维码链接
func (na *Native) Prepay(payment *paytype.Payment) (string, error) {
	svc := native.NativeApiService{Client: na.client}
	req := native.PrepayRequest{
		Appid:       core.String(na.cfg.AppID),        // appid
		Mchid:       core.String(na.cfg.MchID),        // 商户号
		Description: core.String(payment.Description), // 商品描述
		OutTradeNo:  core.String(payment.TradeNo),     // 自定义订单编号
		TimeExpire:  payment.ExpireTime,               // 交易结束时间
		NotifyUrl:   core.String(na.cfg.NotifyURL),    // 回调地址
		Amount: &native.Amount{
			Currency: core.String("CNY"),                            // CNY：人民币，境内商户号仅支持人民币。
			Total:    core.Int64(int64(payment.Amount.Val() * 100)), // 单位为分 1元应填写100分
		},
	}
	resp, _, err := svc.Prepay(na.ctx, req)
	if err != nil {
		// 处理错误
		logs.Errorf("call Prepay err:%s", err)
		return "", err
	}
	payment.PrePayReq, err = paytype.JsonString(req)
	if err != nil {
		logs.Errorf("call Prepay err:%s", err)
		return "", err
	}
	payment.AppID = na.cfg.AppID
	payment.MchID = na.cfg.MchID
	return *resp.CodeUrl, nil
}

// QueryByTradeNo 根据支付号查询订单
func (na *Native) QueryByTradeNo(payment *paytype.Payment) (*QueryResp, error) {
	svc := native.NativeApiService{Client: na.client}
	resp, result, err := svc.QueryOrderByOutTradeNo(na.ctx,
		native.QueryOrderByOutTradeNoRequest{
			OutTradeNo: core.String(payment.TradeNo),
			Mchid:      core.String(na.cfg.MchID),
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

	queryResp := &QueryResp{
		State: *resp.TradeState,
	}
	if queryResp.State == "SUCCESS" {
		payment.PrePayResp, err = paytype.JsonString(resp)
		if err != nil {
			logs.Errorf("call QueryOrderByOutTradeNo err:%s", err)
			return nil, err
		}
		// 使用 time.Parse 解析时间字符串
		parsedTime, err := time.Parse(time.RFC3339, *resp.SuccessTime)
		if err != nil {
			logs.Errorf("Failed to parse time: %v", err)
			return nil, err
		}
		queryResp.SuccessTime = &parsedTime
		queryResp.TransactionId = *resp.TransactionId
	}
	return queryResp, nil
}

// CloseOrder 关闭订单
func (na *Native) CloseOrder(payment *paytype.Payment) error {
	svc := native.NativeApiService{Client: na.client}
	result, err := svc.CloseOrder(na.ctx,
		native.CloseOrderRequest{
			OutTradeNo: core.String(payment.TradeNo),
			Mchid:      core.String(na.cfg.MchID),
		},
	)
	if err != nil {
		// 处理错误
		logs.Errorf("call QueryOrderByOutTradeNo err:%v", err)
		return err
	}
	if result.Response.StatusCode != 204 {
		logs.Errorf("call QueryOrderByOutTradeNo err:%v", result.Response.Status)
		return fmt.Errorf("close order err %v", result.Response.StatusCode)
	}
	return nil
}
