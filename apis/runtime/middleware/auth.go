package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
)

// AuthMiddleWare .
func AuthMiddleWare(ctx *gin.Context) {
	ls := runtime.LoginStatus(ctx)
	if ls.State != auth.StateSucc {
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
		ctx.AbortWithStatusJSON(200, gin.H{
			"code":    401,
			"message": "unauthorized",
		})
		return
	}
	ctx.Next()
}
