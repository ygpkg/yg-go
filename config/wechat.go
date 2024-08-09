package config

import (
	offConfig "github.com/silenceper/wechat/v2/officialaccount/config"
)

// WechatOfficialAccountConfig 微信公众号配置
type WechatOfficialAccountConfig struct {
	offConfig.Config `yaml:",inline"`

	Templates map[string]string `yaml:"templates"`
}
