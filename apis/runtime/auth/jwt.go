package auth

import (
	"context"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
)

// GetJwtSetting 获取jwt配置
func GetJwtSetting(ctx context.Context, issuer string) (*config.JwtConfig, error) {
	jset := &config.JwtConfig{}
	err := settings.GetYaml(ctx, "core", "jwt-"+issuer, jset)
	if err != nil {
		logs.Warnw("[manager_auth] get jwt setting failed.",
			"error", err, "issuer", issuer)
		return nil, err
	}
	return jset, nil
}

// GetJwtSecret 获取jwt密钥
func GetJwtSecret(ctx context.Context, issuer string) ([]byte, error) {
	jset, err := GetJwtSetting(ctx, issuer)
	if err != nil {
		return []byte(""), err
	}
	return []byte(jset.Secret), nil
}
