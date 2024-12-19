package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/logs"
)

// AuthMiddleWare .
func AuthMiddleWare(ctx *gin.Context) {
	ls := runtime.LoginStatus(ctx)
	if ls.State != auth.StateSucc {
		logs.WarnContextf(ctx, "user %s not login", ls.Claim.Uin)
		ctx.AbortWithStatusJSON(200, gin.H{
			"code":    401,
			"message": "unauthorized",
		})
		return
	}
	ctx.Next()
}

// AuthMiddleWareEmployee .
func AuthMiddleWareEmployee(ctx *gin.Context) {
	ls := runtime.LoginStatus(ctx)
	if ls.State != auth.StateSucc || ls.Role != auth.RoleEmployee {
		logs.WarnContextf(ctx, "user %s not employee or not login", ls.Claim.Uin)
		ctx.AbortWithStatusJSON(200, gin.H{
			"code":    401,
			"message": "unauthorized",
		})
		return
	}
	ctx.Next()
}

// AuthMiddleWareCustomer .
func AuthMiddleWareCustomer(ctx *gin.Context) {
	ls := runtime.LoginStatus(ctx)
	if ls.State != auth.StateSucc || ls.Role != auth.RoleCustomer {
		logs.WarnContextf(ctx, "user %s not customer or not login", ls.Claim.Uin)
		ctx.AbortWithStatusJSON(200, gin.H{
			"code":    401,
			"message": "unauthorized",
		})
		return
	}
	ctx.Next()
}
