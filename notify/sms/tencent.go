package sms

import (
	"fmt"
	"os"

	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common"
	"github.com/tencentcloud/tencentcloud-sdk-go/tencentcloud/common/profile"
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
		os.Getenv("TENCENTCLOUD_SECRET_ID"),
		os.Getenv("TENCENTCLOUD_SECRET_KEY"),
	)
	cpf := profile.NewClientProfile()
	cpf.HttpProfile.ReqMethod = "POST"
	cpf.HttpProfile.Endpoint = cfg.Tencent.Endpoint
}
