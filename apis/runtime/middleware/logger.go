package middleware

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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
	reqBodyMaxSize = 128

	// ctxKeyIdentityFields 标记 IdentityFields 已把身份字段写进请求日志上下文。
	// 访问日志（Logger）用它判断是否还要兜底补 uin，不能改用“LoginStatus 在不在”：
	// 那样“挂了 IdentityFields、没挂 LoginStatus”的组合会让同一行出现两个 uin。
	ctxKeyIdentityFields = "yg-go-log-identity-fields"
)

// Logger 访问日志中间件
//
// 脱敏约定（原文来自 canvas2 vendor 的同一份补丁，此处为唯一真源）：
//   - 三个分享接口（video.PrepareShareReference / ConfirmShareReference /
//     DiscardShareReference）的报文里带签名 URL 与私密文案，落进 CLS 就等于泄露，
//     因此 reqbody 固定记 [REDACTED]，uri 只记 URL.Path（丢掉 query 里的 token），
//     referer 记空串；同时用 http.MaxBytesReader 给请求体加 64KB 上限，
//     避免这些接口被超大报文拖垮。
//   - /media.UploadFile 的 multipart 请求体是流式的，读一次就把请求破坏掉（下游
//     io.ReadAll 拿不到内容），所以只记 [STREAMED]，不去读 body。
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
		// Share text can contain signed platform URLs and private prose; the
		// other two actions carry a temporary reference. This logger runs
		// before application middleware and must not record any of them.
		streamedMediaUpload := strings.HasSuffix(currReq, "/media.UploadFile") &&
			strings.HasPrefix(strings.ToLower(ctx.GetHeader("Content-Type")), "multipart/form-data")
		sensitive := strings.HasSuffix(currReq, "/video.PrepareShareReference") ||
			strings.HasSuffix(currReq, "/video.ConfirmShareReference") ||
			strings.HasSuffix(currReq, "/video.DiscardShareReference")
		if sensitive && ctx.Request.Body != nil {
			ctx.Request.Body = http.MaxBytesReader(ctx.Writer, ctx.Request.Body, 64*1024)
		}
		reqBody := "[REDACTED]"
		if streamedMediaUpload {
			reqBody = "[STREAMED]"
		} else if !sensitive {
			var getBodyErr error
			reqBody, getBodyErr = getReqBody(ctx)
			if getBodyErr != nil {
				ctx.Error(getBodyErr)
			}
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
			}).Buckets(0.2, 0.8, 1.6, 5, 10, 60).
			Observe(cost.Seconds())
		requestURI, referer := ctx.Request.RequestURI, ctx.Request.Referer()
		if sensitive {
			requestURI, referer = ctx.Request.URL.Path, ""
		}
		logArgs := []interface{}{
			"method", ctx.Request.Method,
			"uri", requestURI,
			"reqbody", reqBody,
			"reqsize", ctx.Request.ContentLength,
			"latency", fmt.Sprintf("%.3f", cost.Seconds()),
			"clientip", runtime.GetRealIP(ctx.Request),
			"respsize", ctx.Writer.Size(),
			"referer", referer,
		}
		// 身份字段由 IdentityFields 写进日志上下文，两边同源。这里只给“单独使用
		// Logger、没挂 IdentityFields”的裸引擎兜底旧的 uin 输出，否则同一行两个 uin。
		if _, ok := ctx.Get(ctxKeyIdentityFields); !ok {
			logArgs = append(logArgs, "uin", ctx.GetUint(constants.CtxKeyUin))
		}
		if v := ctx.Request.Header.Get("App-Version"); v != "" {
			logArgs = append(logArgs, "appversion", v)
		}
		if v := ctx.Request.Header.Get("App-Type"); v != "" {
			logArgs = append(logArgs, "apptype", v)
		}

		if ctx.Writer.Status() >= 500 {
			logs.LoggerFromContext(ctx).Errorw(fmt.Sprint(ctx.Writer.Status()), logArgs...)
		} else {
			code := ctx.GetInt(constants.CtxKeyCode)
			logs.LoggerFromContext(ctx).Infow(fmt.Sprint(code), logArgs...)
		}
	}
}

// IdentityFields 把身份类字段写进请求日志上下文，访问日志与业务日志共用这一份。
// 恒定输出：未认证写 0 —— companyid="0" 这类过滤条件才不会漏掉匿名流量。
//
// 必须排在 svr.Inject 之后：companyid/uin 来自 LoginStatus.idmap，由各 app 的
// auth injector 在 Inject 里填；放早了读出来恒为 0，而且不会报错。
// 裸引擎（没跑框架中间件）上没有 LoginStatus，runtime.* 会 MustGet panic，故先探测。
func IdentityFields() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		companyID, uin := uint(0), uint(0)
		if _, ok := ctx.Get(constants.CtxKeyLoginStatus); ok {
			companyID, uin = runtime.CompanyID(ctx), runtime.Uin(ctx)
		}
		logs.SetContextFields(ctx, "companyid", companyID, "uin", uin)
		ctx.Set(ctxKeyIdentityFields, true)
		ctx.Next()
	}
}

// RequestLogFields 把 resolve 返回的键值对追加进请求日志上下文。
// 必须挂在 IdentityFields 之后、路由处理之前；resolver 只做纯计算（主链路上同步执行，
// 不要查库/IO），也不得输出框架已有的键（reqid / uin / companyid / uri / method），
// 否则同一行会出现两个同名键。
func RequestLogFields(resolve func(*gin.Context) []interface{}) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if resolve != nil {
			if fields := resolve(ctx); len(fields) > 0 {
				if len(fields)%2 != 0 {
					logs.Warnf("RequestLogFields: odd number of arguments, trailing key dropped")
					fields = fields[:len(fields)-1]
				}
				logs.SetContextFields(ctx, fields...)
			}
		}
		ctx.Next()
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
