package estool

import (
	"crypto/tls"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
)

// InitES 初始化ES
func InitES(cfg config.ESConfig) (*elasticsearch.Client, error) {
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
