package middleware

import (
	"fmt"
	"strings"

	"github.com/dgrijalva/jwt-go"
	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/logs"
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
			if ls.Claim != nil && ls.Claim.Uin > 0 {
				ctx.Set(constants.CtxKeyUin, ls.Claim.Uin)
			}
			// logs.Debugf("login status: %+v", ls)
		}()
		if authstr == "" {
			return
		}

		authstr = strings.TrimPrefix(authstr, auth.AuthBearer)
		authstr = strings.TrimSpace(authstr)
		ls.Token = authstr
		if strings.HasPrefix(authstr, auth.AuthAPIKeyPrefix) {
			ls.Role = auth.RoleAPI
			ls.State = auth.StateSucc
			claims := new(auth.UserClaims)
			ls.Claim = claims
			return
		}

		claims := new(auth.UserClaims)
		_, err := jwt.ParseWithClaims(ls.Token, claims, func(token *jwt.Token) (interface{}, error) {
			if token.Claims == nil {
				return nil, fmt.Errorf("token claims is nil")
			}
			c, ok := token.Claims.(*auth.UserClaims)
			if !ok {
				return nil, fmt.Errorf("token claims is not UserClaims")
			}

			return auth.GetJwtSecret(c.Issuer)
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
