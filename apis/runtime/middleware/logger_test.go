package middleware

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/logs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const bizLogMarker = "handler-biz-log"

// newBufferLogger 构造一个写入 buf 的 zap logger；中间件里用 SetContextLogger 换上去，
// 框架 Logger 在链尾通过 LoggerFromContext 读到它，从而把访问日志捕获到 buf。
func newBufferLogger(buf *bytes.Buffer) *zap.SugaredLogger {
	cfg := zap.NewProductionEncoderConfig()
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(cfg),
		zapcore.AddSync(buf),
		zap.InfoLevel,
	)
	return zap.New(core).Sugar()
}

// captureLogger 把当前请求的 logger 换成写 buf 的实例。必须排在 Logger 之后、
// 身份字段写入之前，否则 SetContextLogger 会覆盖掉已经写入 context 的字段。
func captureLogger(buf *bytes.Buffer) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		logs.SetContextLogger(ctx, newBufferLogger(buf))
		ctx.Next()
	}
}

// newInjectedLoginStatus 构造一个已由注入器填好 idmap 的 LoginStatus。
func newInjectedLoginStatus(companyID, uin uint) *auth.LoginStatus {
	ls := &auth.LoginStatus{}
	ls.SetID(constants.CtxKeyCompanyID, companyID)
	ls.SetID(constants.CtxKeyUin, uin)
	return ls
}

func outputLines(t *testing.T, buf *bytes.Buffer) []string {
	t.Helper()
	var lines []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

func lineWith(t *testing.T, lines []string, substr string) string {
	t.Helper()
	for _, line := range lines {
		if strings.Contains(line, substr) {
			return line
		}
	}
	t.Fatalf("no log line contains %q, got: %v", substr, lines)
	return ""
}

// assertKeyOnce 断言一行日志里某个键只出现一次，钉住“同一行两个 uin/companyid”。
func assertKeyOnce(t *testing.T, line, key string) {
	t.Helper()
	if n := strings.Count(line, `"`+key+`":`); n != 1 {
		t.Fatalf("key %q appears %d times in log line: %s", key, n, line)
	}
}

// 验收 3：未挂 LoginStatus 的裸引擎上单独使用 Logger，访问日志仍带 uin（兜底分支），不 panic。
func TestLoggerFallsBackToCtxUinWithoutIdentityFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buf := &bytes.Buffer{}

	eng := gin.New()
	eng.Use(Logger())
	eng.Use(captureLogger(buf))
	eng.POST("/v1/ping.Ping", func(ctx *gin.Context) {
		ctx.Set(constants.CtxKeyUin, uint(9))
		ctx.String(http.StatusOK, "pong")
	})

	eng.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/ping.Ping", nil))

	access := lineWith(t, outputLines(t, buf), `"uri":`)
	assertKeyOnce(t, access, "uin")
	if !strings.Contains(access, `"uin":9`) {
		t.Fatalf("expected fallback uin=9 in access log, got: %s", access)
	}
}

// IdentityFields 在裸引擎（没有 LoginStatus）上必须先探测再取值，否则 runtime.* 会 panic；
// 且匿名请求要恒定输出 companyid=0 / uin=0。
func TestIdentityFieldsToleratesMissingLoginStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buf := &bytes.Buffer{}

	eng := gin.New()
	eng.Use(Logger())
	eng.Use(captureLogger(buf))
	eng.Use(IdentityFields())
	eng.POST("/v1/ping.Ping", func(ctx *gin.Context) {
		logs.InfoContextf(ctx, bizLogMarker)
		ctx.String(http.StatusOK, "pong")
	})

	eng.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/ping.Ping", nil))

	lines := outputLines(t, buf)
	access := lineWith(t, lines, `"uri":`)
	assertKeyOnce(t, access, "companyid")
	assertKeyOnce(t, access, "uin")
	if !strings.Contains(access, `"companyid":0`) || !strings.Contains(access, `"uin":0`) {
		t.Fatalf("expected anonymous identity fields in access log, got: %s", access)
	}

	biz := lineWith(t, lines, bizLogMarker)
	assertKeyOnce(t, biz, "companyid")
	assertKeyOnce(t, biz, "uin")
}

// 验收 2 的纯中间件版本：handler 内 logs.*Contextf 的输出同样带身份字段。
func TestIdentityFieldsReachHandlerBusinessLog(t *testing.T) {
	gin.SetMode(gin.TestMode)
	buf := &bytes.Buffer{}

	eng := gin.New()
	eng.Use(Logger())
	eng.Use(captureLogger(buf))
	eng.Use(func(ctx *gin.Context) {
		// 模拟 app 注入器：在 IdentityFields 之前把 companyid 写进 idmap。
		ctx.Set(constants.CtxKeyLoginStatus, newInjectedLoginStatus(uint(42), uint(7)))
		ctx.Next()
	})
	eng.Use(IdentityFields())
	eng.POST("/v1/ping.Ping", func(ctx *gin.Context) {
		logs.InfoContextf(ctx, bizLogMarker)
		ctx.String(http.StatusOK, "pong")
	})

	eng.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/ping.Ping", nil))

	lines := outputLines(t, buf)
	access := lineWith(t, lines, `"uri":`)
	if !strings.Contains(access, `"companyid":42`) || !strings.Contains(access, `"uin":7`) {
		t.Fatalf("expected companyid=42 uin=7 in access log, got: %s", access)
	}
	assertKeyOnce(t, access, "companyid")
	assertKeyOnce(t, access, "uin")

	biz := lineWith(t, lines, bizLogMarker)
	if !strings.Contains(biz, `"companyid":42`) || !strings.Contains(biz, `"uin":7`) {
		t.Fatalf("expected companyid=42 uin=7 in business log, got: %s", biz)
	}
	assertKeyOnce(t, biz, "companyid")
	assertKeyOnce(t, biz, "uin")
}

func TestRequestLogFields(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("appends resolved fields", func(t *testing.T) {
		buf := &bytes.Buffer{}
		eng := gin.New()
		eng.Use(Logger())
		eng.Use(captureLogger(buf))
		eng.Use(IdentityFields())
		eng.Use(RequestLogFields(func(ctx *gin.Context) []interface{} {
			return []interface{}{"tenanttype", "corp", "ua", "test-agent"}
		}))
		eng.POST("/v1/ping.Ping", func(ctx *gin.Context) {
			logs.InfoContextf(ctx, bizLogMarker)
			ctx.String(http.StatusOK, "pong")
		})

		eng.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/ping.Ping", nil))

		lines := outputLines(t, buf)
		access := lineWith(t, lines, `"uri":`)
		for _, kv := range []string{`"tenanttype":"corp"`, `"ua":"test-agent"`} {
			if !strings.Contains(access, kv) {
				t.Fatalf("expected %s in access log, got: %s", kv, access)
			}
		}
		biz := lineWith(t, lines, bizLogMarker)
		for _, kv := range []string{`"tenanttype":"corp"`, `"ua":"test-agent"`} {
			if !strings.Contains(biz, kv) {
				t.Fatalf("expected %s in business log, got: %s", kv, biz)
			}
		}
	})

	t.Run("nil resolver is skipped", func(t *testing.T) {
		buf := &bytes.Buffer{}
		eng := gin.New()
		eng.Use(Logger())
		eng.Use(captureLogger(buf))
		eng.Use(IdentityFields())
		eng.Use(RequestLogFields(nil))
		eng.POST("/v1/ping.Ping", func(ctx *gin.Context) {
			logs.InfoContextf(ctx, bizLogMarker)
			ctx.String(http.StatusOK, "pong")
		})

		eng.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/ping.Ping", nil))

		lineWith(t, outputLines(t, buf), bizLogMarker)
	})

	t.Run("odd fields drop the trailing key and keep the rest", func(t *testing.T) {
		buf := &bytes.Buffer{}
		eng := gin.New()
		eng.Use(Logger())
		eng.Use(captureLogger(buf))
		eng.Use(IdentityFields())
		eng.Use(RequestLogFields(func(ctx *gin.Context) []interface{} {
			return []interface{}{"tenanttype", "corp", "orphan"}
		}))
		eng.POST("/v1/ping.Ping", func(ctx *gin.Context) {
			logs.InfoContextf(ctx, bizLogMarker)
			ctx.String(http.StatusOK, "pong")
		})

		eng.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/v1/ping.Ping", nil))

		biz := lineWith(t, outputLines(t, buf), bizLogMarker)
		if !strings.Contains(biz, `"tenanttype":"corp"`) {
			t.Fatalf("expected surviving pair in business log, got: %s", biz)
		}
		if strings.Contains(biz, "orphan") {
			t.Fatalf("trailing key must be dropped for odd fields, got: %s", biz)
		}
	})
}

// 脱敏（P0 回源）：三个分享接口的报文含签名 URL 与私密文案，不能进日志；
// body 也不能被 Logger 读走。这是 canvas2 share_reference_log_test.go 的等价物。
func TestLoggerRedactsShareReferenceRequests(t *testing.T) {
	const secret = "share-sensitive-signed-value"
	for _, action := range []string{"PrepareShareReference", "ConfirmShareReference", "DiscardShareReference"} {
		t.Run(action, func(t *testing.T) {
			gin.SetMode(gin.TestMode)
			buf := &bytes.Buffer{}
			route := "/v1/video." + action

			eng := gin.New()
			eng.Use(Logger())
			eng.Use(captureLogger(buf))
			eng.POST(route, func(ctx *gin.Context) {
				body, err := io.ReadAll(ctx.Request.Body)
				if err != nil {
					t.Errorf("read request body: %v", err)
				}
				if !strings.Contains(string(body), secret) {
					t.Errorf("logger consumed request body, got: %s", string(body))
				}
				ctx.String(http.StatusOK, "ok")
			})

			request := httptest.NewRequest(http.MethodPost, route+"?token="+secret,
				strings.NewReader(`{"request":{"shareText":"`+secret+`"}}`))
			request.Header.Set("Referer", "https://example.invalid/?token="+secret)
			eng.ServeHTTP(httptest.NewRecorder(), request)

			access := lineWith(t, outputLines(t, buf), `"uri":`)
			if !strings.Contains(access, "[REDACTED]") {
				t.Fatalf("expected [REDACTED] in access log, got: %s", access)
			}
			if strings.Contains(access, secret) {
				t.Fatalf("share data leaked in access log: %s", access)
			}
			if !strings.Contains(access, `"uri":"`+route+`"`) {
				t.Fatalf("expected bare path uri in access log, got: %s", access)
			}
			if !strings.Contains(access, `"referer":""`) {
				t.Fatalf("expected empty referer in access log, got: %s", access)
			}
		})
	}
}

// /media.UploadFile 的 multipart 是流式的，读一次会破坏请求，只记 [STREAMED]。
func TestLoggerMarksStreamedMediaUpload(t *testing.T) {
	const secret = "media-sensitive-payload"
	gin.SetMode(gin.TestMode)
	buf := &bytes.Buffer{}
	route := "/v1/media.UploadFile"

	var payload bytes.Buffer
	mw := multipart.NewWriter(&payload)
	part, err := mw.CreateFormFile("file", "share.txt")
	if err != nil {
		t.Fatalf("create multipart form file: %v", err)
	}
	if _, err := part.Write([]byte(secret)); err != nil {
		t.Fatalf("write multipart payload: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	eng := gin.New()
	eng.Use(Logger())
	eng.Use(captureLogger(buf))
	eng.POST(route, func(ctx *gin.Context) {
		body, err := io.ReadAll(ctx.Request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		if !strings.Contains(string(body), secret) {
			t.Errorf("logger consumed request body, got: %s", string(body))
		}
		ctx.String(http.StatusOK, "ok")
	})

	request := httptest.NewRequest(http.MethodPost, route, &payload)
	request.Header.Set("Content-Type", mw.FormDataContentType())
	eng.ServeHTTP(httptest.NewRecorder(), request)

	access := lineWith(t, outputLines(t, buf), `"uri":`)
	if !strings.Contains(access, "[STREAMED]") {
		t.Fatalf("expected [STREAMED] in access log, got: %s", access)
	}
	if strings.Contains(access, secret) {
		t.Fatalf("streamed media payload leaked in access log: %s", access)
	}
}
