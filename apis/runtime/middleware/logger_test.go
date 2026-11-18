package middleware_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/apis/runtime/middleware"
	"github.com/ygpkg/yg-go/apis/runtime/server"
	"github.com/ygpkg/yg-go/logs"
)

// newBufferLogger 构造一条写内存 buffer 的 zap logger，用例靠它读日志原文。
// 不用子进程抓 stdout：把 logger 塞进请求上下文后，框架在链尾也是读同一份。
func newBufferLogger(buf *bytes.Buffer) *zap.SugaredLogger {
	cfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "lvl",
		NameKey:        "mod",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
	}
	core := zapcore.NewCore(zapcore.NewJSONEncoder(cfg), zapcore.AddSync(buf), zap.InfoLevel)
	return zap.New(core).Sugar()
}

// captureLogger 把当前请求的 logger 换成写 buffer 的实例。
// 必须排在 Logger 之后：Logger 先 SetContextFields(reqid)，之后的 SetContextFields
// （IdentityFields、业务扩展点、handler 内的 logs.*Contextf）才都链在这条 logger 上。
func captureLogger(l *zap.SugaredLogger) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		logs.SetContextLogger(ctx, l)
		ctx.Next()
	}
}

// withCaptureLogger 把捕获中间件插到引擎中间件链最前面。
// 必须走 RouterOption：NewRouter 先装框架中间件（含 IdentityFields），再应用 opts，
// 最后才建 routeGroups —— svr.P 注册路由时用的是那一刻快照下来的 handler 链。
// 所以链首注入只能在 opt 里做，放到 NewRouter 返回之后做已经晚了（路由里不含捕获）。
// 也不能像普通中间件那样追加到链尾：排在 IdentityFields 之后会把已经挂上
// companyid/uin 的 logger 整个换掉，身份字段全丢。
func withCaptureLogger(l *zap.SugaredLogger) server.RouterOption {
	return func(svr *server.Router) {
		eng := svr.GinEngine()
		eng.Handlers = append(gin.HandlersChain{captureLogger(l)}, eng.Handlers...)
	}
}

// TestRouterAccessLogCarriesIdentityFields 对应验收 1 / 2 / 6：
// 裸 NewRouter + 一条路由，访问日志与 handler 内业务日志都带 companyid/uin，
// 每个键只出现一次，匿名写 0；模拟 app 注入器 SetID(companyid, 42) 后能读到 42
// —— 这就钉住了「IdentityFields 必须排在 svr.Inject 之后」。
func TestRouterAccessLogCarriesIdentityFields(t *testing.T) {
	buf := &bytes.Buffer{}
	sugar := newBufferLogger(buf)

	svr := server.NewRouter("/v1", withCaptureLogger(sugar))
	// 等价于 canvas2 apps/business/internal/auth 里的注入器；用 API Key（yg- 前缀）
	// 走 StateSucc 分支，避免为了过鉴权在用例里造 JWT。
	svr.AuthInject("", func(ctx *gin.Context, ls *auth.LoginStatus) error {
		ls.SetID(constants.CtxKeyCompanyID, 42)
		ls.SetID(constants.CtxKeyUin, 7)
		return nil
	})
	svr.P("identity.Echo", func(ctx *gin.Context) {
		logs.InfoContextf(ctx, "handler-log-ok")
		ctx.String(http.StatusOK, "ok")
	})
	eng := svr.GinEngine()

	serve := func(authorization string) string {
		buf.Reset()
		req := httptest.NewRequest(http.MethodPost, "/v1/identity.Echo", nil)
		if authorization != "" {
			req.Header.Set("Authorization", authorization)
		}
		eng.ServeHTTP(httptest.NewRecorder(), req)
		return buf.String()
	}

	t.Run("anonymous", func(t *testing.T) {
		out := serve("")
		access := linesWithMsg(t, out, "0")
		if len(access) != 1 {
			t.Fatalf("want 1 access log line, got %d:\n%s", len(access), out)
		}
		assertFieldOnce(t, access[0], "companyid", "0")
		assertFieldOnce(t, access[0], "uin", "0")

		// 验收 2：handler 内的业务日志拿到同一份字段
		handlerLog := linesWithMsg(t, out, "handler-log-ok")
		if len(handlerLog) != 1 {
			t.Fatalf("want 1 handler log line, got %d:\n%s", len(handlerLog), out)
		}
		assertFieldOnce(t, handlerLog[0], "companyid", "0")
		assertFieldOnce(t, handlerLog[0], "uin", "0")
	})

	t.Run("authenticated", func(t *testing.T) {
		out := serve("Bearer yg-test-api-key")
		access := linesWithMsg(t, out, "0")
		if len(access) != 1 {
			t.Fatalf("want 1 access log line, got %d:\n%s", len(access), out)
		}
		// 验收 6：42/7 来自 svr.Inject 里注入器的 SetID，放错位置就是 0
		assertFieldOnce(t, access[0], "companyid", "42")
		assertFieldOnce(t, access[0], "uin", "7")

		handlerLog := linesWithMsg(t, out, "handler-log-ok")
		if len(handlerLog) != 1 {
			t.Fatalf("want 1 handler log line, got %d:\n%s", len(handlerLog), out)
		}
		assertFieldOnce(t, handlerLog[0], "companyid", "42")
		assertFieldOnce(t, handlerLog[0], "uin", "7")
	})
}

// TestLoggerWithoutIdentityFieldsKeepsUin 对应验收 3：裸引擎上单独用 Logger
// （没挂 LoginStatus / IdentityFields）仍然输出 uin，走 logArgs 兜底分支且不 panic。
func TestLoggerWithoutIdentityFieldsKeepsUin(t *testing.T) {
	buf := &bytes.Buffer{}
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.Use(middleware.Logger())
	eng.Use(captureLogger(newBufferLogger(buf)))
	eng.Use(func(ctx *gin.Context) {
		// 模拟只有 LoginStatus 才会写的那份 gin key（uint，GetUint 才读得到）
		ctx.Set(constants.CtxKeyUin, uint(7))
	})
	eng.POST("/v1/demo.Echo", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/v1/demo.Echo", nil)
	eng.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	access := linesWithMsg(t, out, "0")
	if len(access) != 1 {
		t.Fatalf("want 1 access log line, got %d:\n%s", len(access), out)
	}
	assertFieldOnce(t, access[0], "uin", "7")
	if strings.Contains(access[0], `"companyid"`) {
		t.Fatalf("bare engine must not invent companyid: %s", access[0])
	}
}

// TestIdentityFieldsWithoutLoginStatus 钉住兜底判定的依据：有了 IdentityFields 但没有
// LoginStatus 时，IdentityFields 写 0，Logger 不再追加 logArgs 里的 uin，不会出现两个 uin。
func TestIdentityFieldsWithoutLoginStatus(t *testing.T) {
	buf := &bytes.Buffer{}
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.Use(middleware.Logger())
	eng.Use(captureLogger(newBufferLogger(buf)))
	eng.Use(middleware.IdentityFields())
	eng.POST("/v1/demo.Echo", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })

	req := httptest.NewRequest(http.MethodPost, "/v1/demo.Echo", nil)
	eng.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	access := linesWithMsg(t, out, "0")
	if len(access) != 1 {
		t.Fatalf("want 1 access log line, got %d:\n%s", len(access), out)
	}
	assertFieldOnce(t, access[0], "companyid", "0")
	assertFieldOnce(t, access[0], "uin", "0")
}

// TestRequestLogFields 覆盖具名扩展点：正常字段进上下文、nil resolver 与奇数个参数不出错。
func TestRequestLogFields(t *testing.T) {
	cases := []struct {
		name    string
		resolve func(*gin.Context) []interface{}
		want    []string
		dropped string
	}{
		{
			name:    "nil resolver",
			resolve: nil,
		},
		{
			name: "empty fields",
			resolve: func(*gin.Context) []interface{} {
				return nil
			},
		},
		{
			name: "extra fields",
			resolve: func(*gin.Context) []interface{} {
				return []interface{}{"tenanttype", "vip", "ua", "curl"}
			},
			want: []string{`"tenanttype":"vip"`, `"ua":"curl"`},
		},
		{
			// 奇数个参数：丢掉最后一个（落单的 key），其余照常写入
			name: "odd fields",
			resolve: func(*gin.Context) []interface{} {
				return []interface{}{"tenanttype", "vip", "lonely-key"}
			},
			want:    []string{`"tenanttype":"vip"`},
			dropped: "lonely-key",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			buf := &bytes.Buffer{}
			gin.SetMode(gin.TestMode)
			eng := gin.New()
			eng.Use(middleware.Logger())
			eng.Use(captureLogger(newBufferLogger(buf)))
			eng.Use(middleware.IdentityFields())
			eng.Use(middleware.RequestLogFields(c.resolve))
			eng.POST("/v1/demo.Echo", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })

			req := httptest.NewRequest(http.MethodPost, "/v1/demo.Echo", nil)
			eng.ServeHTTP(httptest.NewRecorder(), req)

			out := buf.String()
			access := linesWithMsg(t, out, "0")
			if len(access) != 1 {
				t.Fatalf("want 1 access log line, got %d:\n%s", len(access), out)
			}
			assertFieldOnce(t, access[0], "companyid", "0")
			for _, want := range c.want {
				if !strings.Contains(access[0], want) {
					t.Fatalf("access log missing %s: %s", want, access[0])
				}
			}
			if c.dropped != "" && strings.Contains(access[0], c.dropped) {
				t.Fatalf("trailing key %q must be dropped: %s", c.dropped, access[0])
			}
		})
	}
}

// TestLoggerRedactsShareReference 是 canvas2
// apps/business/internal/apis/videocreationctl/share_reference_log_test.go 的等价物：
// 三个分享接口的请求体、query token、Referer 都不能进日志，body 也不能被 logger 吃掉。
func TestLoggerRedactsShareReference(t *testing.T) {
	const secret = "share-sensitive-signed-value"

	for _, action := range []string{"PrepareShareReference", "ConfirmShareReference", "DiscardShareReference"} {
		t.Run(action, func(t *testing.T) {
			buf := &bytes.Buffer{}
			gin.SetMode(gin.TestMode)
			eng := gin.New()
			eng.Use(middleware.Logger())
			eng.Use(captureLogger(newBufferLogger(buf)))

			route := "/v1/video." + action
			eng.POST(route, func(ctx *gin.Context) {
				body, err := io.ReadAll(ctx.Request.Body)
				if err != nil || !strings.Contains(string(body), secret) {
					t.Errorf("logger consumed request body: err=%v body=%q", err, body)
				}
				ctx.String(http.StatusOK, "ok")
			})

			req := httptest.NewRequest(http.MethodPost, route+"?token="+secret,
				strings.NewReader(`{"request":{"shareText":"`+secret+`"}}`))
			req.Header.Set("Referer", "https://example.invalid/?token="+secret)
			eng.ServeHTTP(httptest.NewRecorder(), req)

			out := buf.String()
			access := linesWithMsg(t, out, "0")
			if len(access) != 1 {
				t.Fatalf("want 1 access log line, got %d:\n%s", len(access), out)
			}
			if !strings.Contains(access[0], "[REDACTED]") {
				t.Fatalf("want [REDACTED] in access log: %s", access[0])
			}
			if strings.Contains(out, secret) {
				t.Fatalf("share data leaked into log: %s", out)
			}
			// uri 只留 path，query 里的 token 与 referer 一并丢掉
			if !strings.Contains(access[0], `"uri":"`+route+`"`) {
				t.Fatalf("want uri without query: %s", access[0])
			}
			if !strings.Contains(access[0], `"referer":""`) {
				t.Fatalf("want empty referer: %s", access[0])
			}
		})
	}
}

// TestLoggerStreamedMediaUpload 钉住流式上传：multipart 的 /media.UploadFile 只记
// [STREAMED]，不读 body（读一次下游就拿不到文件了）。
func TestLoggerStreamedMediaUpload(t *testing.T) {
	const secret = "streamed-upload-body"

	buf := &bytes.Buffer{}
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.Use(middleware.Logger())
	eng.Use(captureLogger(newBufferLogger(buf)))
	eng.POST("/v1/media.UploadFile", func(ctx *gin.Context) {
		body, err := io.ReadAll(ctx.Request.Body)
		if err != nil || !strings.Contains(string(body), secret) {
			t.Errorf("logger consumed request body: err=%v body=%q", err, body)
		}
		ctx.String(http.StatusOK, "ok")
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/media.UploadFile", strings.NewReader(secret))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=----yg-go")
	eng.ServeHTTP(httptest.NewRecorder(), req)

	out := buf.String()
	access := linesWithMsg(t, out, "0")
	if len(access) != 1 {
		t.Fatalf("want 1 access log line, got %d:\n%s", len(access), out)
	}
	if !strings.Contains(access[0], "[STREAMED]") {
		t.Fatalf("want [STREAMED] in access log: %s", access[0])
	}
	if strings.Contains(out, secret) {
		t.Fatalf("upload body leaked into log: %s", out)
	}
}

// linesWithMsg 取出 msg 字段等于给定值的日志行（原文，用于数重复键）。
func linesWithMsg(t *testing.T, out, msg string) []string {
	t.Helper()
	needle := `"msg":"` + msg + `"`
	var res []string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, needle) {
			res = append(res, line)
		}
	}
	return res
}

// assertFieldOnce 断言一行里 key 只出现一次，且取值等于 want。
func assertFieldOnce(t *testing.T, line, key, want string) {
	t.Helper()
	if n := strings.Count(line, `"`+key+`"`); n != 1 {
		t.Fatalf("field %q appears %d times, want 1: %s", key, n, line)
	}
	if !strings.Contains(line, `"`+key+`":`+want) {
		t.Fatalf("field %q != %s: %s", key, want, line)
	}
}
