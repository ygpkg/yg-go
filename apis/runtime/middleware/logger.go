package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/metrics"
)

const (
	reqBodyMaxSize = 1024
)

// Logger .
func Logger(whitelist ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		reqid := ctx.GetString(constants.CtxKeyRequestID)

		logs.SetContextFields(ctx, "reqid", reqid)
		currReq := ctx.FullPath()
		for _, whitelistItem := range whitelist {
			if strings.HasSuffix(currReq, whitelistItem) {
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
			}).Buckets(0.1, 0.3, 0.5, 1, 3, 5, 10, 30, 60, 300).
			Observe(cost.Seconds())
		if ctx.Writer.Status() >= 400 && ctx.Writer.Status() != 401 {
			logs.LoggerFromContext(ctx).Errorw(fmt.Sprint(ctx.Writer.Status()),
				"method", ctx.Request.Method,
				"uri", ctx.Request.RequestURI,
				"reqbody", reqBody,
				"reqsize", ctx.Request.ContentLength,
				"latency", fmt.Sprintf("%.3f", cost.Seconds()),
				"clientip", runtime.GetRealIP(ctx.Request),
				"respsize", ctx.Writer.Size(),
				"referer", ctx.Request.Referer(),
				"uin", ctx.GetUint(constants.CtxKeyUin),
			)
		} else {
			code := ctx.GetInt(constants.CtxKeyCode)
			logs.LoggerFromContext(ctx).Infow(fmt.Sprint(code),
				"method", ctx.Request.Method,
				"uri", ctx.Request.RequestURI,
				"reqbody", reqBody,
				"reqsize", ctx.Request.ContentLength,
				"latency", fmt.Sprintf("%.3f", cost.Seconds()),
				"clientip", runtime.GetRealIP(ctx.Request),
				"respsize", ctx.Writer.Size(),
				"referer", ctx.Request.Referer(),
				"uin", ctx.GetUint(constants.CtxKeyUin),
			)
		}
	}
}

func getReqBody(ctx *gin.Context) (string, error) {
	if ctx.Request.Body == nil {
		return "", nil
	}
	byteBody, err := ctx.GetRawData()
	if err != nil {
		return "", err
	}
	ctx.Request.Body = io.NopCloser(bytes.NewBuffer(byteBody))

	// 尝试压缩 JSON
	var compacted bytes.Buffer
	if json.Valid(byteBody) {
		err := json.Compact(&compacted, byteBody)
		if err == nil {
			return compacted.String(), nil
		}
	}
	// 非 JSON 或解析失败，原样返回
	return string(byteBody), nil
}
