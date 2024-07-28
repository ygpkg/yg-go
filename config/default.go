package config

import "go.uber.org/zap/zapcore"

// DefaultConfig 默认配置
var DefaultConfig = CoreConfig{
	MainConf: MainConfig{
		HttpAddr:    ":8080",
		OpenDocsAPI: true,
		Env:         "dev",
	},
	LogsConf: LogsConfig{
		"main": []LogConfig{
			{Level: zapcore.InfoLevel}, // Logger: &lumberjack.Logger{
			// 	Filename:   "./logs/roc.log",
			// 	MaxSize:    100,
			// 	MaxBackups: 3,
			// 	MaxAge:     7,
			// },
		},
		"access": []LogConfig{
			{Level: zapcore.InfoLevel}, // Logger: &lumberjack.Logger{
			// 	Filename:   "./logs/access.log",
			// 	MaxSize:    100,
			// 	MaxBackups: 3,
			// 	MaxAge:     7,
			// },
		},
	},
}
