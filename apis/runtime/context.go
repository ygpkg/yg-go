package runtime

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"go.uber.org/zap"
)

func Logger(ctx *gin.Context) *zap.SugaredLogger {
	return ctx.MustGet(constants.CtxKeyLogger).(*zap.SugaredLogger)
}

func RequestID(ctx *gin.Context) string {
	return ctx.MustGet(constants.CtxKeyRequestID).(string)
}

func LoginStatus(ctx *gin.Context) *auth.LoginStatus {
	return ctx.MustGet(constants.CtxKeyLoginStatus).(*auth.LoginStatus)
}

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
