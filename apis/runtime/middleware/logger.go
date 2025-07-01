package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/logs"
)

const (
	reqBodyMaxSize = 1024
)

// Logger .
func Logger() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		reqid := ctx.GetString(constants.CtxKeyRequestID)

		logs.SetContextFields(ctx, "reqid", reqid)
		currReq := fmt.Sprintf("%s %s", ctx.Request.Method, ctx.FullPath())
		for _, whitelistItem := range []string{
			"POST /apis/zmdevice/v1/ping",
			"POST /apis/zmdevice/v1/task_logs",
			"POST /apis/p/zmrobot.KeepTaskHeartbeat",
		} {
			if whitelistItem == currReq {
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
