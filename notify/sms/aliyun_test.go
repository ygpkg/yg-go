package sms

import (
	"testing"

	"github.com/ygpkg/yg-go/config"
)

func TestSendVerifyCodeByAliyun(t *testing.T) {
	if true {
		t.Skip()
		return
	}

	cfg := &config.SMSConfig{
		Aliyun: &config.AliConfig{
			AccessKeyID:     "",
			AccessKeySecret: "",
		},
		TemplateCode: "",
		SignName:     "言古科技",
	}
	err := sendVerifyCodeByAliyun(cfg, "", "321456")
	if err != nil {
		t.Error(err)
	}

}
