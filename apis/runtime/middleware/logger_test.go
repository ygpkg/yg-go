package middleware

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/logs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// capture 把当前请求的 logger 换成写 buffer 的实例，必须排在 Logger 之前。
func capture(buf *bytes.Buffer) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		core := zapcore.NewCore(
			zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
			zapcore.AddSync(buf),
			zapcore.InfoLevel,
		)
		logs.SetContextLogger(ctx, zap.New(core).Sugar())
		ctx.Next()
	}
}

func newEngine(buf *bytes.Buffer, mws ...gin.HandlerFunc) *gin.Engine {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.Use(capture(buf))
	eng.Use(mws...)
	eng.POST("/v1/probe", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })
	return eng
}

func serve(eng *gin.Engine) {
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/probe", strings.NewReader(`{}`)))
}

// TestApplyLoginStatusWritesIDMap 钉住 v1.23.80 的回归点：
// 只把 uin 写进 gin key 时 runtime.Uin 恒为 0（idmap 只有 injector 会填）。
func TestApplyLoginStatusWritesIDMap(t *testing.T) {
	ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
	ls := &auth.LoginStatus{Claim: &auth.UserClaims{Uin: 7}}

	applyLoginStatus(ctx, ls)

	if got := ctx.GetUint(constants.CtxKeyUin); got != 7 {
		t.Fatalf("gin key 上的 uin = %d，want 7", got)
	}
	if got := runtime.Uin(ctx); got != 7 {
		t.Fatalf("runtime.Uin = %d，want 7（只写 gin key 会让它恒为 0）", got)
	}
}

// TestContextFieldsDefaultsToNoOp 钉住"默认不变"：不传选项时一个字段都不写。
func TestContextFieldsDefaultsToNoOp(t *testing.T) {
	var buf bytes.Buffer
	eng := newEngine(&buf, ContextFields())
	serve(eng)

	if strings.Contains(buf.String(), "companyid") || strings.Contains(buf.String(), "uin") {
		t.Fatalf("未开启身份字段时不应写入任何身份键：%s", buf.String())
	}
}

// TestLoggerDefaultKeepsUinInLogArgs 钉住老行为：不开身份字段时，uin 仍由 logArgs 输出。
func TestLoggerDefaultKeepsUinInLogArgs(t *testing.T) {
	var buf bytes.Buffer
	eng := newEngine(&buf, Logger())
	serve(eng)

	if n := strings.Count(buf.String(), `"uin"`); n != 1 {
		t.Fatalf("访问日志里 uin 出现 %d 次，want 1：%s", n, buf.String())
	}
	if strings.Contains(buf.String(), "companyid") {
		t.Fatalf("未开启身份字段时不应有 companyid：%s", buf.String())
	}
}

// TestLoggerWithIdentityFieldsDedupes 钉住身份字段与 logArgs 不重复。
func TestLoggerWithIdentityFieldsDedupes(t *testing.T) {
	var buf bytes.Buffer
	eng := newEngine(&buf,
		LoggerWithOptions(WithIdentityFields(true)),
		ContextFields(WithIdentityFields(true)),
	)
	serve(eng)

	line := buf.String()
	for _, key := range []string{"uin", "companyid"} {
		if n := strings.Count(line, `"`+key+`"`); n != 1 {
			t.Fatalf("%q 出现 %d 次，want 1：%s", key, n, line)
		}
	}
}

// TestContextFieldsIdentityFromIDMap：注入器写入的身份要出现在访问日志里。
func TestContextFieldsIdentityFromIDMap(t *testing.T) {
	var buf bytes.Buffer
	eng := newEngine(&buf,
		LoggerWithOptions(WithIdentityFields(true)),
		// 模拟框架 Inject + app 注入器
		func(ctx *gin.Context) {
			ls := &auth.LoginStatus{Claim: &auth.UserClaims{Uin: 7}}
			ls.SetID(constants.CtxKeyUin, 7)
			ls.SetID(constants.CtxKeyCompanyID, 42)
			ctx.Set(constants.CtxKeyLoginStatus, ls)
			ctx.Next()
		},
		ContextFields(WithIdentityFields(true)),
	)
	serve(eng)

	line := buf.String()
	if !strings.Contains(line, `"companyid":42`) || !strings.Contains(line, `"uin":7`) {
		t.Fatalf("身份字段未进访问日志：%s", line)
	}
}

// TestContextFieldsUinFallsBackToClaim：injector 漏写 uin 时退回 claim 通道。
func TestContextFieldsUinFallsBackToClaim(t *testing.T) {
	var buf bytes.Buffer
	eng := newEngine(&buf,
		LoggerWithOptions(WithIdentityFields(true)),
		func(ctx *gin.Context) {
			ls := &auth.LoginStatus{Claim: &auth.UserClaims{Uin: 9}}
			ctx.Set(constants.CtxKeyLoginStatus, ls)
			ctx.Set(constants.CtxKeyUin, uint(9))
			ctx.Next()
		},
		ContextFields(WithIdentityFields(true)),
	)
	serve(eng)

	if n := strings.Count(buf.String(), `"uin":9`); n != 1 {
		t.Fatalf("uin 未回退到 claim 通道（出现 %d 次）：%s", n, buf.String())
	}
}

// TestWithRequestIDHeader：回写响应头 + 追加 CORS 暴露头。
func TestWithRequestIDHeader(t *testing.T) {
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.Use(func(ctx *gin.Context) {
		ctx.Header("Access-Control-Expose-Headers", "Content-Length")
		ctx.Set(constants.CtxKeyRequestID, "rid-1")
	})
	eng.Use(LoggerWithOptions(WithRequestIDHeader("X-Request-Id", true)))
	eng.POST("/v1/probe", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })

	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/probe", strings.NewReader(`{}`)))

	if got := rec.Header().Get("X-Request-Id"); got != "rid-1" {
		t.Fatalf("X-Request-Id = %q，want rid-1", got)
	}
	if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "Content-Length, X-Request-Id" {
		t.Fatalf("expose 头 = %q", got)
	}
}
