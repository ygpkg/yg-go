package config

import "time"

// ESConfig ES配置
type ESConfig struct {
	Addresses     []string
	Username      string
	Password      string
	MaxRetries    int           // 最大重试次数
	SlowThreshold time.Duration // 慢查询阈值，示例 100ms
}
