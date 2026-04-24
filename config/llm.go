package config

// ProxyConfig holds proxy connection settings including scheme, address, and credentials.
type ProxyConfig struct {
	Scheme   string `yaml:"scheme"`
	Addr     string `yaml:"addr"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// LLMModelConfig holds LLM model selection and parameter options.
type LLMModelConfig struct {
	Provider    string       `json:"provider" yaml:"provider"`
	APIKey      string       `json:"api_key" yaml:"api_key"`
	BaseURL     string       `json:"base_url" yaml:"base_url"`
	ModelName   string       `json:"model_name" yaml:"model_name"`
	Proxy       *ProxyConfig `json:"proxy,omitempty" yaml:"proxy,omitempty"`
	Temperature float32      `json:"temperature,omitempty" yaml:"temperature,omitempty"`
	MaxTokens   int          `json:"max_tokens,omitempty" yaml:"max_tokens,omitempty"`
	TopP        float32      `json:"top_p,omitempty" yaml:"top_p,omitempty"`
}
