package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/logs"
)

// loginStatus 从 ctx 取登录态；中间件被关闭（自研账号体系）时返回 nil，而不是 MustGet panic。
func loginStatus(ctx *gin.Context) *auth.LoginStatus {
	value, ok := ctx.Get(constants.CtxKeyLoginStatus)
	if !ok {
		return nil
	}
	ls, _ := value.(*auth.LoginStatus)
	return ls
}

// AuthMiddleWare 要求登录态。未登录（含 LoginStatus 中间件被关闭）一律 401，不 panic。
func AuthMiddleWare(ctx *gin.Context) {
	ls := loginStatus(ctx)
	if ls == nil || ls.State != auth.StateSucc {
		logs.WarnContextf(ctx, "user login state invalid")
		ctx.AbortWithStatusJSON(200, gin.H{
			"code":    401,
			"message": "unauthorized",
		})
		return
	}
	ctx.Next()
}

// AuthMiddleWareEmployee 要求员工身份。未登录（含 LoginStatus 中间件被关闭）一律 401，不 panic。
func AuthMiddleWareEmployee(ctx *gin.Context) {
	ls := loginStatus(ctx)
	if ls == nil || ls.State != auth.StateSucc || ls.Role != auth.RoleEmployee {
		logs.WarnContextf(ctx, "user not employee or not login")
		ctx.AbortWithStatusJSON(200, gin.H{
			"code":    401,
			"message": "unauthorized",
		})
		return
	}
	ctx.Next()
}
