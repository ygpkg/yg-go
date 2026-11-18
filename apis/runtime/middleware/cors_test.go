package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// 本文件钉住 CORS 选项的语义：每个选项对应一个实际扩展场景（来源策略、请求头、暴露头、
// 凭证、预检缓存），默认行为与 1.23 线的 middleware.CORS() 一致。

func newCORSEngine(t *testing.T, opts ...CORSOption) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	eng := gin.New()
	eng.Use(CORS(opts...))
	eng.POST("/x", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })
	return eng
}

func corsDo(eng *gin.Engine, method string, hdrs map[string]string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/x", nil)
	for key, value := range hdrs {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	eng.ServeHTTP(rec, req)
	return rec
}

// TestCORSDefaults 默认配置：允许所有来源、允许携带凭证（与 1.23 线一致）。
func TestCORSDefaults(t *testing.T) {
	rec := corsDo(newCORSEngine(t), http.MethodPost, map[string]string{"Origin": "https://app.example.com"})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("Allow-Origin = %q, want *", got)
	}
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
		t.Fatalf("Allow-Credentials = %q, want true", got)
	}
}

// TestCORSAllowOrigins 指定来源：命中来源回显、未命中被拒。
func TestCORSAllowOrigins(t *testing.T) {
	eng := newCORSEngine(t, WithCORSAllowOrigins("https://app.example.com", "https://admin.example.com"))

	rec := corsDo(eng, http.MethodPost, map[string]string{"Origin": "https://app.example.com"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Fatalf("allowed origin: Allow-Origin = %q", got)
	}
	rec = corsDo(eng, http.MethodPost, map[string]string{"Origin": "https://evil.example.com"})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("disallowed origin status = %d, want 403", rec.Code)
	}
}

// TestCORSAllowOriginFunc 动态来源：函数返回 true 才放行（多环境/子域名场景）。
func TestCORSAllowOriginFunc(t *testing.T) {
	eng := newCORSEngine(t, WithCORSAllowOriginFunc(func(origin string) bool {
		return strings.HasSuffix(origin, ".example.com") || origin == "https://example.com"
	}))
	rec := corsDo(eng, http.MethodPost, map[string]string{"Origin": "https://tenant1.example.com"})
	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://tenant1.example.com" {
		t.Fatalf("allowed origin = %q, want echo", got)
	}
	if rec := corsDo(eng, http.MethodPost, map[string]string{"Origin": "https://other.test"}); rec.Code != http.StatusForbidden {
		t.Fatalf("disallowed origin status = %d, want 403", rec.Code)
	}
}

// TestCORSAllowHeaders 追加自定义请求头（预检里能看到默认头 + 自定义头）。
func TestCORSAllowHeaders(t *testing.T) {
	eng := newCORSEngine(t, WithCORSAllowHeaders("X-Tenant-Id", "X-Client-Version"))
	rec := corsDo(eng, http.MethodOptions, map[string]string{
		"Origin":                         "https://app.example.com",
		"Access-Control-Request-Method":  http.MethodPost,
		"Access-Control-Request-Headers": "X-Tenant-Id",
	})
	if rec.Code != http.StatusNoContent {
		t.Fatalf("preflight status = %d, want 204", rec.Code)
	}
	allowHeaders := rec.Header().Get("Access-Control-Allow-Headers")
	for _, want := range []string{"X-Tenant-Id", "X-Client-Version", "Authorization", "Content-Type"} {
		if !strings.Contains(allowHeaders, want) {
			t.Fatalf("Allow-Headers = %q, missing %q", allowHeaders, want)
		}
	}
}

// TestCORSExposeHeaders 追加暴露给浏览器的响应头。
func TestCORSExposeHeaders(t *testing.T) {
	eng := newCORSEngine(t, WithCORSExposeHeaders("X-Request-Id"))
	expose := corsDo(eng, http.MethodPost, map[string]string{"Origin": "https://app.example.com"}).
		Header().Get("Access-Control-Expose-Headers")
	if !strings.Contains(expose, "X-Request-Id") || !strings.Contains(expose, "Content-Length") {
		t.Fatalf("Expose-Headers = %q, want X-Request-Id + Content-Length", expose)
	}
}

// TestCORSAllowCredentialsAndMaxAge 凭证开关 + 预检缓存时间。
func TestCORSAllowCredentialsAndMaxAge(t *testing.T) {
	eng := newCORSEngine(t, WithCORSAllowCredentials(false), WithCORSMaxAge(time.Minute))
	rec := corsDo(eng, http.MethodOptions, map[string]string{
		"Origin":                        "https://app.example.com",
		"Access-Control-Request-Method": http.MethodPost,
	})
	if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
		t.Fatalf("Allow-Credentials = %q, want empty", got)
	}
	if got := rec.Header().Get("Access-Control-Max-Age"); got != "60" {
		t.Fatalf("Max-Age = %q, want 60", got)
	}
}

// TestCORSAllowMethods 覆盖允许方法。
func TestCORSAllowMethods(t *testing.T) {
	eng := newCORSEngine(t, WithCORSAllowMethods(http.MethodGet, http.MethodPost))
	allowMethods := corsDo(eng, http.MethodOptions, map[string]string{
		"Origin":                        "https://app.example.com",
		"Access-Control-Request-Method": http.MethodPost,
	}).Header().Get("Access-Control-Allow-Methods")
	if allowMethods != "GET,POST" {
		t.Fatalf("Allow-Methods = %q, want GET,POST", allowMethods)
	}

	// 零参保持默认方法集（与 WithCORSAllowOrigins 零参「回到默认」语义一致）。
	eng = newCORSEngine(t, WithCORSAllowMethods())
	allowMethods = corsDo(eng, http.MethodOptions, map[string]string{
		"Origin":                        "https://app.example.com",
		"Access-Control-Request-Method": http.MethodPost,
	}).Header().Get("Access-Control-Allow-Methods")
	if !strings.Contains(allowMethods, "GET") || !strings.Contains(allowMethods, "POST") {
		t.Fatalf("zero-arg AllowMethods should keep defaults, got %q", allowMethods)
	}
}

// TestCORSAllowOriginFuncNilPanics nil 校验在应用选项时 panic，避免静默失效。
func TestCORSAllowOriginFuncNilPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic for nil origin func")
		}
	}()
	_ = CORS(WithCORSAllowOriginFunc(nil))
}
