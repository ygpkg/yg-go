package middleware

import (
	"fmt"
	"strings"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
)

// LoginStatus 注入用户登录状态
func LoginStatus() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var (
			authstr = ctx.Request.Header.Get("Authorization")
			ls      = &auth.LoginStatus{}
		)
		defer func() {
			ctx.Set(constants.CtxKeyLoginStatus, ls)
			// logs.Debugf("login status: %+v", ls)
		}()
		if authstr == "" {
			return
		}

		authstr = strings.TrimPrefix(authstr, auth.AuthBearer)
		authstr = strings.TrimSpace(authstr)
		ls.Token = authstr

		claims := new(auth.UserClaims)
		_, err := jwt.ParseWithClaims(ls.Token, claims, func(token *jwt.Token) (interface{}, error) {
			if token.Claims == nil {
				return nil, fmt.Errorf("token claims is nil")
			}
			c, ok := token.Claims.(*auth.UserClaims)
			if !ok {
				return nil, fmt.Errorf("token claims is not UserClaims")
			}

			return getJwtSetting(c.Issuer)
		})
		if err != nil {
			logs.Warnw("[manager_auth] parse claims failed.",
				"error", err, "token", ls.Token)
			ls.Err = err
			ls.State = auth.StateFailed
			return
		}
		ls.State = auth.StateSucc
		ls.Claim = claims
	}
}

// getJwtSetting 获取jwt配置
func getJwtSetting(issuer string) ([]byte, error) {
	jset := &config.JwtConfig{}
	err := settings.GetYaml("core", "jwt-"+issuer, jset)
	if err != nil {
		logs.Warnw("[manager_auth] get jwt setting failed.",
			"error", err, "issuer", issuer)
		return []byte(""), err
	}
	return []byte(jset.Secret), nil
}
