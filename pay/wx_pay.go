package pay

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/wechatpay-apiv3/wechatpay-go/core"
	"github.com/wechatpay-apiv3/wechatpay-go/core/auth/verifiers"
	"github.com/wechatpay-apiv3/wechatpay-go/core/downloader"
	"github.com/wechatpay-apiv3/wechatpay-go/core/notify"
	"github.com/wechatpay-apiv3/wechatpay-go/core/option"
	"github.com/wechatpay-apiv3/wechatpay-go/services/payments"
	"github.com/wechatpay-apiv3/wechatpay-go/services/refunddomestic"
	"github.com/wechatpay-apiv3/wechatpay-go/utils"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/pay/paytype"
	"github.com/ygpkg/yg-go/settings"
	"gorm.io/gorm"
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
func NewWxPay(pay_type, group, key string) (Pay, error) {
	var (
		ctx = context.Background()
		cfg = &config.WXPayConfig{}
	)
	err := settings.GetYaml(group, key, &cfg)
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
func WxRefund(ctx context.Context, payment *paytype.Payment, refund *paytype.PayRefund, group, key string) error {
	var (
		cfg = &config.WXPayConfig{}
	)
	err := settings.GetYaml(group, key, &cfg)
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
func WxQueryRefund(ctx context.Context, refund *paytype.PayRefund, group, key string) (*QueryResp, error) {
	var (
		cfg = &config.WXPayConfig{}
	)
	err := settings.GetYaml(group, key, &cfg)
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

// WxNotify 微信支付回调
func WxNotify(db *gorm.DB, ctx *gin.Context, group, key string) error {
	var (
		cfg = &config.WXPayConfig{}
	)
	err := settings.GetYaml(group, key, &cfg)
	if err != nil {
		logs.Errorf("get wxpay config error: %v", err)
		return err
	}
	// 加载私钥生成签名
	mchPrivateKey, err := utils.LoadPrivateKey(cfg.Pemkey)
	if err != nil {
		logs.Errorf("load private key err:%v", err)
		return err
	}
	// 1. 使用 `RegisterDownloaderWithPrivateKey` 注册下载器
	err = downloader.MgrInstance().RegisterDownloaderWithPrivateKey(ctx, mchPrivateKey, cfg.MchCertificateSerialNumber, cfg.MchID, cfg.MchAPIv3Key)
	if err != nil {
		logs.Errorf("RegisterDownloaderWithPrivateKey err:%v", err)
		return err
	}
	// 2. 获取商户号对应的微信支付平台证书访问器
	certificateVisitor := downloader.MgrInstance().GetCertificateVisitor(cfg.MchID)
	// 3. 使用证书访问器中的证书进行验签
	handler := notify.NewNotifyHandler(cfg.MchAPIv3Key, verifiers.NewSHA256WithRSAVerifier(certificateVisitor))
	transaction := new(payments.Transaction)
	notifyReq, err := handler.ParseNotifyRequest(ctx, ctx.Request, transaction)
	// 如果验签未通过，或者解密失败
	if err != nil {
		logs.Errorf("ParseNotifyRequest err:%v", err)
		return err
	}
	logs.Infof("ParseNotifyRequest success notifyReq:%v,transaction:%v,", notifyReq, transaction)
	// 搜索支付表信息
	payment, err := paytype.GetPayPaymentByTradeNo(db, *transaction.OutTradeNo)
	if err != nil {
		logs.Errorf("get pay payment by trade no err:%v", err)
		return err
	}
	// 使用 time.Parse 解析时间字符串
	parsedTime, err := time.Parse(time.RFC3339, *transaction.SuccessTime)
	if err != nil {
		logs.Errorf("Failed to parse time: %v", err)
		return err
	}
	// 处理回调请求
	if notifyReq.EventType == "TRANSACTION.SUCCESS" {
		if payment.PayStatus == paytype.PayStatusSuccess {
			return nil
		}
		// 支付成功
		payment.PayStatus = paytype.PayStatusSuccess
		payment.PrePayResp, err = paytype.JsonString(transaction)
		if err != nil {
			logs.Errorf("call QueryOrderByOutTradeNo err:%s", err)
			return err
		}

		payment.PaySuccessTime = &parsedTime
		payment.TransactionID = *transaction.TransactionId
		order, err := paytype.GetPayOrderByOrderNo(db, payment.OrderNo)
		if err != nil {
			logs.Errorf("QueryByTradeNo GetPayOrderByOrderNo failed,err=%v", err)
			return err
		}
		order.OrderStatus = paytype.OrderStatusPendingSend
		order.PayStatus = paytype.PayStatusSuccess
		order.PayType = payment.PayType
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
			logs.Errorf("wxnotifi Transaction failed,err=%v", err)
			return err
		}
		// 删除支付号key
		err = DeleteTradeNoKey(context.Background(), order.OrderNo)
		if err != nil {
			logs.Errorf("QueryByTradeNo DeleteTradeNoKey failed,err=%v", err)
			return err
		}
	}
	if notifyReq.EventType == "REFUND.SUCCESS" {
		// 退款成功
		refund, err := paytype.GetPayRefundByStatus(db, payment.OrderNo, paytype.PayStatusPendingRefund)
		if err != nil {
			logs.Errorf("QueryByTradeNo GetPayRefund failed,err=%v", err)
			return err
		}
		if refund.PayStatus == paytype.PayStatusSuccessRefund {
			return nil
		}
		refund.PayStatus = paytype.PayStatusSuccessRefund
		refund.RefundSuccessTime = &parsedTime
		order, err := paytype.GetPayOrderByOrderNo(db, refund.OrderNo)
		if err != nil {
			logs.Errorf("QueryRefund GetPayOrderByOrderNo failed,err=%v", err)
			return err
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
			return err
		}
		// 删除支付号key
		err = DeleteRefundNoKey(context.Background(), order.OrderNo)
		if err != nil {
			logs.Errorf("QueryRefund DeleteRefundNoKey failed,err=%v", err)
			return err
		}
	}
	return nil
}
