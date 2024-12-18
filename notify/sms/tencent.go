package sms

import (
	"fmt"

	"github.com/ygpkg/yg-go/config"
)

func sendVerifyCodeByTencent(cfg *config.SMSConfig, phone string, code string) error {
	if cfg.Tencent == nil {
		return fmt.Errorf("tencent config is empty")
	}
	if cfg.Tencent.SecretID == "" || cfg.Tencent.SecretKey == "" {
		return fmt.Errorf("tencent secret_id or secret_key is empty")
	}

}
