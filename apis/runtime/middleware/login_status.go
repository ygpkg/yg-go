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

// applyLoginStatus 把解析结果落到请求上。
//
// 两个地方都要写：gin key（供 ctx.GetUint 之类的老读法）与 LoginStatus.idmap
// （供 runtime.Uin / runtime.CompanyID）。**只写 gin key 会让 runtime.Uin 恒为 0**——
// idmap 只有各 app 的 injector 会填，没有 injector 的服务就永远读不到 claim 里的 uin。
// 这是 v1.23.80 那次改动的回归点，单独提出来是为了能被用例直接钉住。
func applyLoginStatus(ctx *gin.Context, ls *auth.LoginStatus) {
	ctx.Set(constants.CtxKeyLoginStatus, ls)
	if ls.Claim != nil && ls.Claim.Uin > 0 {
		ctx.Set(constants.CtxKeyUin, ls.Claim.Uin)
		ls.SetID(constants.CtxKeyUin, ls.Claim.Uin)
	}
}

// LoginStatus 注入用户登录状态
func LoginStatus() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		var (
			authstr = ctx.Request.Header.Get("Authorization")
			ls      = &auth.LoginStatus{}
		)
		defer func() {
			applyLoginStatus(ctx, ls)
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

			return auth.GetJwtSecretWithCtx(ctx, c.Issuer)
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
		ls.Issuer = claims.Issuer
	}
}
