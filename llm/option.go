package llm

// Config holds the driver factory configuration including base URL, proxy, model name, and HTTP client.
type Config struct {
	BaseURL    string
	ProxyURL   string
	ModelName  string
	HTTPClient any
}

// Option is a functional option interface for configuring the LLM driver factory.
type Option interface {
	Apply(*Config)
}

type optionFunc func(*Config)

func (f optionFunc) Apply(cfg *Config) { f(cfg) }

// WithBaseURL sets a custom API base URL for providers compatible with the OpenAI protocol.
func WithBaseURL(baseURL string) Option {
	return optionFunc(func(cfg *Config) {
		cfg.BaseURL = baseURL
	})
}

// WithProxy sets the HTTP proxy URL for outbound requests.
func WithProxy(proxyURL string) Option {
	return optionFunc(func(cfg *Config) {
		cfg.ProxyURL = proxyURL
	})
}

// WithDefaultModel sets the default model name used when the request does not specify one.
func WithDefaultModel(model string) Option {
	return optionFunc(func(cfg *Config) {
		cfg.ModelName = model
	})
}
