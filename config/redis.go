package config

// RedisConfig redis 连接属性
type RedisConfig struct {
	Host        string `yaml:"host" json:"host"`
	Password    string `yaml:"password" json:"password"`
	Database    int    `yaml:"database" json:"database"`
	MaxIdle     int    `yaml:"max_idle" json:"max_idle"`
	MaxActive   int    `yaml:"max_active" json:"max_active"`
	IdleTimeout int32  `yaml:"idle_timeout" json:"idle_timeout"` //second
}
