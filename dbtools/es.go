package dbtools

import (
	"crypto/tls"
	"net/http"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/ygpkg/yg-go/logs"
)

type ESConfig struct {
	Addresses     []string
	Username      string
	Password      string
	MaxRetries    int           // 最大重试次数
	SlowThreshold time.Duration // 慢查询阈值，示例 100ms
}

func InitES(cfg ESConfig) (*elasticsearch.Client, error) {
	logCfg := logs.ESLoggerConfig{
		LoggerName:    "es",
		SlowThreshold: cfg.SlowThreshold,
	}
	esLogger := logs.GetESLogger(logCfg)

	esCfg := elasticsearch.Config{
		Addresses:  cfg.Addresses,
		Username:   cfg.Username,
		Password:   cfg.Password,
		MaxRetries: cfg.MaxRetries,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // 忽略 SSL 证书验证
			},
		},
		Logger: esLogger,
	}
	client, newClientErr := elasticsearch.NewClient(esCfg)
	if newClientErr != nil {
		return nil, newClientErr
	}
	return client, nil
}
