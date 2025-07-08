package middleware

import (
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/ygpkg/yg-go/metrics"
)

// Metrics .
func Metrics(whitelist ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		currPath := ctx.FullPath()
		for _, whitelistItem := range whitelist {
			if strings.HasSuffix(currPath, whitelistItem) {
				ctx.Next()
				return
			}
		}

		reqBody, getBodyErr := getReqBody(ctx)
		if getBodyErr != nil {
			ctx.Error(getBodyErr)
		}
		if len(reqBody) > reqBodyMaxSize {
			reqBody = reqBody[:reqBodyMaxSize]
		}

		start := time.Now()
		ctx.Next()
		cost := time.Since(start)
		metrics.Histogram("request_latency_seconds").
			With(prometheus.Labels{
				"uri":  ctx.Request.URL.Path,
				"code": fmt.Sprint(ctx.Writer.Status()),
			}).Observe(cost.Seconds())
	}
}
