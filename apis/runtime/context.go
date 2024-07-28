package runtime

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/logs"
	"go.uber.org/zap"
)

// Logger 日志
func Logger(ctx *gin.Context) *zap.SugaredLogger {
	return logs.LoggerFromContext(ctx)
}

// RequestID 请求ID
func RequestID(ctx *gin.Context) string {
	return ctx.MustGet(constants.CtxKeyRequestID).(string)
}

// LoginStatus 登录状态
func LoginStatus(ctx *gin.Context) *auth.LoginStatus {
	return ctx.MustGet(constants.CtxKeyLoginStatus).(*auth.LoginStatus)
}

// GetRealIP 平台
func GetRealIP(req *http.Request) string {
	xrel := req.Header.Get("X-Real-Ip")
	if xrel != "" {
		return xrel
	}

	xfor := req.Header.Get("X-Forwarded-For")
	if xfor != "" {
		return xfor
	}

	return req.RemoteAddr
}
