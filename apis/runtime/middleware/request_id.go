package middleware

import (
	"encoding/hex"

	"github.com/gin-gonic/gin"
	uuid "github.com/satori/go.uuid"
	"github.com/ygpkg/yg-go/apis/constants"
)

type idGenerator func() string

// GenerateRequestID generates a unique request ID for each incoming request.
func GenerateRequestID(generator idGenerator) gin.HandlerFunc {
	if generator == nil {
		generator = func() string {
			return hex.EncodeToString(uuid.Must(uuid.NewV4(), nil).Bytes())
		}
	}
	return func(ctx *gin.Context) {
		reqID := ctx.Request.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = generator()
		}
		ctx.Set(constants.CtxKeyRequestID, reqID)
	}
}
