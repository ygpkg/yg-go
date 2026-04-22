package llm

// Config 驱动工厂配置项
type Config struct {
	BaseURL    string
	ProxyURL   string
	ModelName  string
	HTTPClient any
}

// Option 驱动工厂配置选项函数
type Option interface {
	Apply(*Config)
}

type optionFunc func(*Config)

func (f optionFunc) Apply(cfg *Config) { f(cfg) }

// WithBaseURL 设置自定义 API 基地址（兼容 OpenAI 协议的其他厂商如 DeepSeek、千问）
func WithBaseURL(baseURL string) Option {
	return optionFunc(func(cfg *Config) {
		cfg.BaseURL = baseURL
	})
}

// WithProxy 设置 HTTP 代理地址（格式：http://user:pass@host:port 或 socks5://host:port）
func WithProxy(proxyURL string) Option {
	return optionFunc(func(cfg *Config) {
		cfg.ProxyURL = proxyURL
	})
}

// WithDefaultModel 设置默认模型名称（请求中未指定 Model 时使用）
func WithDefaultModel(model string) Option {
	return optionFunc(func(cfg *Config) {
		cfg.ModelName = model
	})
}
