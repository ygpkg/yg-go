package config

// RabbitMQConfig 配置结构体
type RabbitMQConfig struct {
	URL          string `yaml:"url"`
	ExchangeName string `yaml:"exchange_name"`
	ExchangeType string `yaml:"exchange_type"`
	QueueName    string `yaml:"queue_name"`
	RoutingKey   string `yaml:"routing_key"`
}
