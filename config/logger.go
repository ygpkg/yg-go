package config

import (
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// LogsConfig 。
type LogsConfig map[string][]LogConfig

// LogConfig 。
type LogConfig struct {
	// Writer 日志输出位置 console/file/workwx
	Writer string `yaml:"writer"`
	// Encoder 编码格式
	Encoder            string        `yaml:"encoder"`
	Level              zapcore.Level `yaml:"level"`
	Key                string        `yaml:"key,omitempty"`
	*lumberjack.Logger `yaml:",inline"`
}

// Get() 。
func (c LogsConfig) Get(name string) []LogConfig {
	cfg, ok := c[name]
	if !ok {
		return []LogConfig{defaultLogConfig}
	}
	return cfg
}

// Default 。
func (c LogsConfig) Default() []LogConfig {
	for _, name := range []string{"main", "default"} {
		cfg, ok := c[name]
		if !ok {
			break
		}
		return cfg
	}
	return []LogConfig{defaultLogConfig}
}

var defaultLogConfig = LogConfig{
	Writer: "console",
	Level:  zapcore.InfoLevel,
}
