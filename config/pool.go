package config

// ServiceInfo 服务信息
type ServiceInfo struct {
	// Name 服务名称, 用于标识服务, 例如: mysql, redis
	Name string `yaml:"name"`
	// Cap 服务容量
	Cap int `yaml:"cap"`
}

// ServicePoolConfig 服务池配置
type ServicePoolConfig struct {
	// Services 服务配置
	Services []ServiceInfo `yaml:"services"`
}
