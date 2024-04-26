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
