package sms

import (
	"testing"

	"github.com/ygpkg/yg-go/config"
)

func TestSendVerifyCodeByTencent(t *testing.T) {
	if false {
		t.Skip()
		return
	}

	cfg := &config.SMSConfig{
		Tencent: &config.TencentSMSConfig{
			SmsSdkAppId: "",
			TencentConfig: config.TencentConfig{
				SecretID:  "",
				SecretKey: "",
				Region:    "",
				Endpoint:  "",
			},
		},
		// 填模板id
		TemplateCode: "",
		SignName:     "言古科技",
	}
	err := sendVerifyCodeByTencent(cfg, "13333333333", "321456")
	if err != nil {
		t.Error(err)
	}

}
