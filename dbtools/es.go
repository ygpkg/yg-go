package dbtools

import (
	"crypto/tls"
	"net/http"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/ygpkg/yg-go/logs"
)

type ESConfig struct {
	Addr     string `yaml:"addr"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	Sniff    bool   `yaml:"sniff"`
}

func InitES(cfg ESConfig) (*elasticsearch.Client, error) {
	esCfg := elasticsearch.Config{
		Addresses: []string{cfg.Addr},
		Username:  cfg.Username,
		Password:  cfg.Password,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true, // 忽略 SSL 证书验证
			},
		},
		Logger: logs.GetESLogger("es"),
	}
	client, newClientErr := elasticsearch.NewClient(esCfg)
	if newClientErr != nil {
		return nil, newClientErr
	}
	return client, nil
}
