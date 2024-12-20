package sms

import (
	"fmt"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
	"gorm.io/gorm"
)

type SmsSend struct {
	gorm.Model

	Phone      string `gorm:"type:varchar(20);not null;comment:手机号"`
	VerifyCode string `gorm:"type:varchar(10);not null;comment:验证码"`

	ErrMsg string `gorm:"type:varchar(100);comment:错误信息"`

	OutId string `gorm:"type:varchar(100);comment:外部流水扩展字段"`
}

func (SmsSend) TableName() string {
	return "core_sms_send"
}

func InitDB() error {
	return dbtools.Core().AutoMigrate(&SmsSend{})
}

// SendVerifyCode 发送验证码
func SendVerifyCode(group, key, phone, code, outid string) error {
	cfg := &config.SMSConfig{}
	err := settings.GetYaml(group, key, cfg)
	if err != nil {
		return err
	}

	if cfg.Aliyun != nil {
		err = sendVerifyCodeByAliyun(cfg, phone, code)
		if err != nil {
			logs.Errorf("sendVerifyCodeByAliyun failed ,err %s", err)
		}
	} else if cfg.Tencent != nil {
		err = sendVerifyCodeByTencent(cfg, phone, code)
		if err != nil {
			logs.Errorf("sendVerifyCodeByTencent failed ,err %s", err)
		}
	} else {
		err = fmt.Errorf("sms config is empty")
	}

	sms := &SmsSend{
		Phone:      phone,
		VerifyCode: code,
		OutId:      outid,
	}
	if err != nil {
		sms.ErrMsg = err.Error()
	}
	dbtools.Core().Create(sms)

	return err
}
