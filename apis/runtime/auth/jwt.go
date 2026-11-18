package auth

import (
	"context"

	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
)

// jwtSecretResolver 是密钥的兜底来源：settings 里查不到，或查到了但密钥为空时调用。
//
// 为什么需要它：签发方不一定把密钥写进配置中心（常见做法是签发方从环境变量注入，
// 校验方读同一份环境变量）。把兜底做成注册式，而不是在库里写死某个 issuer 的特例。
var jwtSecretResolver func(issuer string, cause error) (secret []byte, ok bool)

// SetJwtSecretResolver 注册 JWT 密钥兜底解析器；传 nil 取消注册。
//
// 只在启动阶段调用一次，运行期只读（不加锁）。解析器返回 (secret, true) 表示采用该密钥，
// 返回 (nil, false) 表示放弃、按原来的错误处理。
func SetJwtSecretResolver(fn func(issuer string, cause error) (secret []byte, ok bool)) {
	jwtSecretResolver = fn
}

// resolveJwtSecret 按注册的兜底解析器取密钥；未注册或放弃时返回 ok=false。
func resolveJwtSecret(issuer string, cause error) ([]byte, bool) {
	if jwtSecretResolver == nil {
		return nil, false
	}
	return jwtSecretResolver(issuer, cause)
}

// GetJwtSetting 获取jwt配置
func GetJwtSetting(issuer string) (*config.JwtConfig, error) {
	return GetJwtSettingWithCtx(context.Background(), issuer)
}

// GetJwtSettingWithCtx 获取jwt配置
func GetJwtSettingWithCtx(ctx context.Context, issuer string) (*config.JwtConfig, error) {
	jset := &config.JwtConfig{}
	err := settings.GetYaml("core", "jwt-"+issuer, jset, settings.WithContext(ctx))
	if err != nil {
		logs.Warnw("[manager_auth] get jwt setting failed.",
			"error", err, "issuer", issuer)
		return nil, err
	}
	return jset, nil
}

// GetJwtSecret 获取jwt密钥
func GetJwtSecret(issuer string) ([]byte, error) {
	return GetJwtSecretWithCtx(context.Background(), issuer)
}

// GetJwtSecretWithCtx 获取jwt密钥
//
// settings 查不到、或密钥为空时，交给 SetJwtSecretResolver 注册的兜底解析器；
// 两条路都拿不到才返回错误/空串。
func GetJwtSecretWithCtx(ctx context.Context, issuer string) ([]byte, error) {
	jset, err := GetJwtSettingWithCtx(ctx, issuer)
	if err != nil {
		if secret, ok := resolveJwtSecret(issuer, err); ok {
			return secret, nil
		}
		return []byte(""), err
	}
	if jset.Secret == "" {
		if secret, ok := resolveJwtSecret(issuer, nil); ok {
			return secret, nil
		}
	}
	return []byte(jset.Secret), nil
}
