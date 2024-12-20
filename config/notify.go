package config

// SMTPConfig 发邮件参数
type SMTPConfig struct {
	Name     string `yaml:"name"`
	Addr     string `yaml:"addr"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Nickname string `yaml:"nickname"`
}

type NotifyConfig struct {
	// SMTP 发邮件参数
	SMTPs     []*SMTPConfig `yaml:"smtps"`
	WecomApps WecomApps     `yaml:"wecom_apps"`
}

// WecomApp 企业微信应用
func (c NotifyConfig) WecomApp(name string) WecomApp {
	for _, app := range c.WecomApps {
		if app.Name == name {
			return app
		}
	}
	return WecomApp{}
}

type SMSConfig struct {
	// Aliyun 阿里云短信
	Aliyun       *AliConfig        `yaml:"aliyun"`
	Tencent      *TencentSMSConfig `yaml:"tencent"`
	SignName     string            `yaml:"sign_name"`
	TemplateCode string            `yaml:"template_code"`
}

// TencentCOSConfig 腾讯云对象存储配置
type TencentSMSConfig struct {
	TencentConfig `yaml:",inline"`
	SmsSdkAppId   string `yaml:"sms_sdk_app_id"`
}

var s = ``
