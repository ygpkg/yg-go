package middleware

import (
	"encoding/hex"

	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/config"
)

// CustomerHeader .
func CustomerHeader() gin.HandlerFunc {
	env := config.Conf().Env
	return func(ctx *gin.Context) {
		if env == "" {
			return
		}
		reqID := ctx.Request.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = hex.EncodeToString(uuid.Must(uuid.NewV4(), nil).Bytes())
		}
		ctx.Set(constants.CtxKeyRequestID, reqID)
		ctx.Writer.Header().Set("Env", env)
	}
}
