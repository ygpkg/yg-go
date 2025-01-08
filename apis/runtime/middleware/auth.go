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
		logs.WarnContextf(ctx, "user login state invalid")
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
		logs.WarnContextf(ctx, "user not employee or not login")
		ctx.AbortWithStatusJSON(200, gin.H{
			"code":    401,
			"message": "unauthorized",
		})
		return
	}
	ctx.Next()
}
