package auth

import (
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
)

// GetJwtSetting 获取jwt配置
func GetJwtSetting(issuer string) (*config.JwtConfig, error) {
	jset := &config.JwtConfig{}
	err := settings.GetYaml("core", "jwt-"+issuer, jset)
	if err != nil {
		logs.Warnw("[manager_auth] get jwt setting failed.",
			"error", err, "issuer", issuer)
		return nil, err
	}
	return jset, nil
}

// GetJwtSetting 获取jwt配置
func GetJwtSecret(issuer string) ([]byte, error) {
	jset := &config.JwtConfig{}
	err := settings.GetYaml("core", "jwt-"+issuer, jset)
	if err != nil {
		logs.Warnw("[manager_auth] get jwt setting failed.",
			"error", err, "issuer", issuer)
		return []byte(""), err
	}
	return []byte(jset.Secret), nil
}
