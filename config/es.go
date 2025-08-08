package config

import "time"

// ESConfig ES配置
type ESConfig struct {
	Addresses     []string      `yaml:"addresses"`
	Username      string        `yaml:"username"`
	Password      string        `yaml:"password"`
	MaxRetries    int           `yaml:"max_retries"`    // 最大重试次数
	SlowThreshold time.Duration `yaml:"slow_threshold"` // 慢查询阈值，示例 100ms
}
