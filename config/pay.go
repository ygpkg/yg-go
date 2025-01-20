package config

// WXPayConfig 微信支付配置
type WXPayConfig struct {
	// 商户号
	MchID string `yaml:"mch_id"`
	// 商户证书序列号
	MchCertificateSerialNumber string `yaml:"mch_certificate_serial_number"`
	// 商户APIv3密钥
	MchAPIv3Key string `yaml:"mch_api_v3_key"`
	// appid
	AppID string `yaml:"app_id"`
	// 密钥
	Pemkey string `yaml:"pemkey"`
	// 回调地址
	NotifyURL string `yaml:"notify_url"`
}
