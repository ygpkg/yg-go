package config

type ChatGPTConfig struct {
	Token      string            `yaml:"token"`
	TokenName  string            `yaml:"token_name"`
	Tokens     map[string]string `yaml:"tokens"`
	HTTPClient HTTPClientConfig  `yaml:"http_client"`
}

type HTTPClientConfig struct {
	Proxy *ProxyConfig `yaml:"proxy"`
}

type ProxyConfig struct {
	Scheme   string `yaml:"scheme"`
	Addr     string `yaml:"addr"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
}

// LLMModelConfig llm 模型选项
type LLMModelConfig struct {
	Proxy *ProxyConfig `yaml:"proxy"`

	APIKEY    string `json:"api_key" yaml:"api_key"`
	BaseURL   string `json:"base_url" yaml:"base_url"`
	ModelName string `json:"model_name" yaml:"model_name"`
}
