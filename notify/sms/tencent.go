package sms

import (
	"fmt"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	sms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
)

func sendVerifyCodeByTencent(cfg *config.SMSConfig, phone string, code string) error {
	if cfg.Tencent == nil {
		logs.Errorf("tencent config is empty")
		return fmt.Errorf("tencent config is empty")
	}
	if cfg.Tencent.SecretID == "" || cfg.Tencent.SecretKey == "" {
		logs.Errorf("tencent secret_id or secret_key is empty")
		return fmt.Errorf("tencent secret_id or secret_key is empty")
	}
	credential := common.NewCredential(
		cfg.Tencent.SecretID,
		cfg.Tencent.SecretKey,
	)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.ReqMethod = "POST"
	// cpf.HttpProfile.Endpoint = cfg.Tencent.Endpoint
	client, _ := sms.NewClient(credential, cfg.Tencent.Region, cpf)
	request := sms.NewSendSmsRequest()
	//  短信签名内容
	request.SmsSdkAppId = common.StringPtr(cfg.Tencent.SmsSdkAppId)
	request.SignName = common.StringPtr(cfg.SignName)
	//  短信模板ID
	request.TemplateId = common.StringPtr(cfg.TemplateCode)
	//  短信模板参数
	request.TemplateParamSet = common.StringPtrs([]string{code})
	// 手机号 最多50个
	request.PhoneNumberSet = common.StringPtrs([]string{phone})
	resp, err := client.SendSms(request)
	if _, ok := err.(*errors.TencentCloudSDKError); ok {
		fmt.Printf("an api error has returned: %s", err)
		return fmt.Errorf("an api error has returned: %s", err)
	}
	if err != nil {
		return err
	}
	for _, v := range resp.Response.SendStatusSet {
		if *v.Code != "Ok" {
			logs.Errorf("send sms failed, code: %s, message: %s", *v.Code, *v.Message)
			return fmt.Errorf("send sms failed, code: %s, message: %s", *v.Code, *v.Message)
		}
	}

	return nil
}
