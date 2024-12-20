package sms

import (
	"fmt"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/errors"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
	sms "github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/sms/v20210111"
	"github.com/ygpkg/yg-go/config"
)

func sendVerifyCodeByTencent(cfg *config.SMSConfig, phone string, code string) error {
	if cfg.Tencent == nil {
		return fmt.Errorf("tencent config is empty")
	}
	if cfg.Tencent.SecretID == "" || cfg.Tencent.SecretKey == "" {
		return fmt.Errorf("tencent secret_id or secret_key is empty")
	}
	credential := common.NewCredential(
		cfg.Tencent.SecretID,
		cfg.Tencent.SecretKey,
	)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.ReqMethod = "POST"
	cpf.HttpProfile.Endpoint = cfg.Tencent.Endpoint
	client, _ := sms.NewClient(credential, "ap-guangzhou", cpf)
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
	_, err := client.SendSms(request)
	if _, ok := err.(*errors.TencentCloudSDKError); ok {
		return fmt.Errorf("an api error has returned: %s", err)
	}
	if err != nil {
		return err
	}
	return nil
}
