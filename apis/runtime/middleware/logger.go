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
	reqBodyMaxSize = 128

	// ctxKeyIdentityFields 标记身份字段已由 ContextFields 写进请求日志上下文。
	// 访问日志用它决定是否还要自己补 uin，避免同一行出现两个同名键。
	ctxKeyIdentityFields = "yg-go-log-identity-fields"
)

// LoggerOption 是访问日志相关中间件的**进程级**配置。
//
// 全部默认关闭：一个选项都不传时，日志行的字段与 v1.23.79 逐字一致——升级不改变
// 任何既有服务的输出。需要新能力（身份字段、业务维度、响应头回写）才显式打开。
type LoggerOption func(*loggerConfig)

type loggerConfig struct {
	whitelist   []string
	identity    bool
	extraFields func(*gin.Context) []interface{}
	reqIDHeader string
	exposeReqID bool
}

// WithWhitelist 设置不打访问日志的路由后缀（框架默认 ".Ping" / "metrics"）。
func WithWhitelist(suffixes ...string) LoggerOption {
	return func(c *loggerConfig) { c.whitelist = suffixes }
}

// WithIdentityFields 打开框架内置身份字段：companyid / uin 恒定写入请求日志上下文
// （未认证写 0），访问日志与请求内业务日志共用同一份。
func WithIdentityFields(on bool) LoggerOption {
	return func(c *loggerConfig) { c.identity = on }
}

// WithExtraFields 注册业务维度解析器，返回值追加进请求日志上下文。
//
// resolver 只做纯计算（主链路上同步执行，不要查库/IO），且不得输出框架已有的键
// （reqid / uin / companyid / uri / method），否则同一行会出现两个同名键。
func WithExtraFields(fn func(*gin.Context) []interface{}) LoggerOption {
	return func(c *loggerConfig) { c.extraFields = fn }
}

// WithRequestIDHeader 让框架回写请求 ID 响应头；expose 为 true 时同时把该头追加进
// CORS 的 Access-Control-Expose-Headers（同源请求不追加）。
func WithRequestIDHeader(name string, expose bool) LoggerOption {
	return func(c *loggerConfig) {
		c.reqIDHeader = name
		c.exposeReqID = expose
	}
}

// Logger 访问日志中间件。
//
// 兼容入口：等价于 LoggerWithOptions(WithWhitelist(whitelist...))。
func Logger(whitelist ...string) gin.HandlerFunc {
	return LoggerWithOptions(WithWhitelist(whitelist...))
}

// LoggerWithOptions 按选项构造访问日志中间件。
//
// 字段来源有三处，缺一不可（改之前先读）：
//   - 请求事实（method/uri/reqbody/…）：本中间件在入口取；
//   - 身份字段（companyid/uin）与业务维度：由 ContextFields 在 svr.Inject 之后写入
//     请求日志上下文，本中间件在链尾通过 logs.LoggerFromContext 读到——所以它也
//     自动出现在 handler 内的 logs.*Contextf 里。
func LoggerWithOptions(opts ...LoggerOption) gin.HandlerFunc {
	cfg := &loggerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(ctx *gin.Context) {
		reqid := ctx.GetString(constants.CtxKeyRequestID)

		logs.SetContextFields(ctx, "reqid", reqid)

		if cfg.reqIDHeader != "" {
			if reqid != "" {
				ctx.Writer.Header().Set(cfg.reqIDHeader, reqid)
			}
			if cfg.exposeReqID {
				appendExposeHeader(ctx.Writer.Header(), cfg.reqIDHeader)
			}
		}

		currReq := ctx.FullPath()
		for _, whitelistItem := range cfg.whitelist {
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
			}).Buckets(0.2, 0.8, 1.6, 5, 10, 60).
			Observe(cost.Seconds())
		logArgs := []interface{}{
			"method", ctx.Request.Method,
			"uri", ctx.Request.RequestURI,
			"reqbody", reqBody,
			"reqsize", ctx.Request.ContentLength,
			"latency", fmt.Sprintf("%.3f", cost.Seconds()),
			"clientip", runtime.GetRealIP(ctx.Request),
			"respsize", ctx.Writer.Size(),
			"referer", ctx.Request.Referer(),
		}
		// 身份字段已由 ContextFields 写进日志上下文时，这里不能再写一遍（同一行两个 uin）。
		// 没开身份字段的服务保持老行为：uin 从 claim 通道取。
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

// ContextFields 把身份字段与业务维度写进请求日志上下文。
//
// **必须排在 svr.Inject 之后**（NewRouter 就是这么装的）：companyid/uin 来自
// LoginStatus.idmap，由各 app 的 auth injector 在 Inject 里填；放早了读出来恒为 0，
// 且不会报错。写入之后，访问日志与 handler 内的 logs.*Contextf 共用同一份字段。
func ContextFields(opts ...LoggerOption) gin.HandlerFunc {
	cfg := &loggerConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	return func(ctx *gin.Context) {
		fields := make([]interface{}, 0, 8)

		if cfg.identity {
			companyID, uin := uint(0), uint(0)
			if _, ok := ctx.Get(constants.CtxKeyLoginStatus); ok {
				companyID, uin = runtime.CompanyID(ctx), runtime.Uin(ctx)
				// 兜底：injector 没把 uin 写进 idmap 时退回 claim 通道，
				// 避免"有 token 却记 uin=0"这种静默回归。
				if uin == 0 {
					uin = ctx.GetUint(constants.CtxKeyUin)
				}
			}
			fields = append(fields, "companyid", companyID, "uin", uin)
			ctx.Set(ctxKeyIdentityFields, true)
		}

		if cfg.extraFields != nil {
			extra := cfg.extraFields(ctx)
			if len(extra)%2 != 0 {
				logs.Warnf("ContextFields: odd number of arguments, trailing key dropped")
				extra = extra[:len(extra)-1]
			}
			fields = append(fields, extra...)
		}

		if len(fields) > 0 {
			logs.SetContextFields(ctx, fields...)
		}
		ctx.Next()
	}
}

// IdentityFields 是随 v1.23.80 导出过的入口，保留为兼容别名。
func IdentityFields() gin.HandlerFunc {
	return ContextFields(WithIdentityFields(true))
}

// appendExposeHeader 把 name 追加进 CORS 暴露头列表（幂等，忽略大小写）。
//
// 只在已经有该头时追加：同源请求本就能读任意响应头，平白加一个只会增加噪声。
func appendExposeHeader(h map[string][]string, name string) {
	const key = "Access-Control-Expose-Headers"
	current := h[key]
	if len(current) == 0 {
		return
	}
	list := current[0]
	for _, item := range strings.Split(list, ",") {
		if strings.EqualFold(strings.TrimSpace(item), name) {
			return
		}
	}
	h[key] = []string{list + ", " + name}
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
