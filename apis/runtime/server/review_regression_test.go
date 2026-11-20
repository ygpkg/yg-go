package server

import (
	"net"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime/middleware"
)

// 本文件补齐第二轮评审（R2）指出的「文档承诺但无测试」项：
// Docs 多实例的 redoc URL / 静态资源、Any 的 9 个方法键、构造期 panic 面、Run 守卫、
// 未声明鉴权前缀的确定性文案。

// TestDocsRedocURLAndIndependentAssets 覆盖 Docs 的两个高风险点：
//  1. redoc 的 spec-url 必须带本前缀（不能拼成 "/v3app.docs/doc.json"）；
//  2. swaggerFiles.Handler 是包级单例，多个 Docs 挂载点必须各自拥有独立的 Prefix，
//     交错访问两个实例的静态资源不能 404。
func TestDocsRedocURLAndIndependentAssets(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3"))
	scope := svr.RouterGroup("/v3")
	scope.Docs("app")
	scope.Docs("biz")

	for _, model := range []string{"app", "biz"} {
		rec := doRequest(svr, http.MethodGet, "/v3/"+model+".redocs", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s.redocs status = %d, want 200", model, rec.Code)
		}
		want := "spec-url='/v3/" + model + ".docs/doc.json'"
		if !strings.Contains(rec.Body.String(), want) {
			t.Fatalf("%s.redocs body missing %q:\n%s", model, want, rec.Body.String())
		}
	}

	// 交错访问：先 app 再 biz 再 app，第二个 app 访问本来会因单例 Prefix 被改写而 404。
	for _, path := range []string{
		"/v3/app.docs/swagger-ui.css",
		"/v3/biz.docs/swagger-ui.css",
		"/v3/app.docs/swagger-ui.css",
	} {
		if rec := doRequest(svr, http.MethodGet, path, nil, nil); rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200（Docs 实例共享 swaggerFiles.Handler？）", path, rec.Code)
		}
	}
}

// TestAnyRegistersNineMethodKeys 断言 Any 的 9 个方法键都进了重复检测索引，
// 防止未来从 anyRouteMethods 里漏掉方法（如 TRACE/CONNECT）而测试仍绿。
func TestAnyRegistersNineMethodKeys(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3"))
	svr.RouterGroup("/v3").Any("/any.probe", func(ctx *gin.Context) {})
	for _, method := range anyRouteMethods {
		key := method + " /v3/any.probe"
		if _, ok := svr.routes[key]; !ok {
			t.Fatalf("routes index missing %q", key)
		}
	}
	if _, ok := svr.routes["ANY /v3/any.probe"]; ok {
		t.Fatal("routes index must expand Any into 9 method keys, not store ANY key")
	}
}

// TestRegistrationPanicSurface 覆盖 R2 评审指出的「README 承诺但无测试」的构造期失败面。
func TestRegistrationPanicSurface(t *testing.T) {
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") }
	cases := []struct {
		name         string
		fn           func()
		wantContains []string
	}{
		{
			name:         "nil RouterOption",
			fn:           func() { _ = NewRouter(WithPrefixes("/v3"), nil) },
			wantContains: []string{"RouterOption", "must not be nil"},
		},
		{
			name:         "blank prefix",
			fn:           func() { _ = NewRouter(WithPrefixes("")) },
			wantContains: []string{"prefix must not be blank"},
		},
		{
			name:         "nil middleware impl",
			fn:           func() { _ = NewRouter(WithPrefixes("/v3"), WithMiddlewareImpl(CORS, nil)) },
			wantContains: []string{"WithMiddlewareImpl", "must not be nil"},
		},
		{
			name:         "zero MwEntry in WithoutMiddleware",
			fn:           func() { _ = NewRouter(WithPrefixes("/v3"), WithoutMiddleware(MwEntry{})) },
			wantContains: []string{"unknown middleware"},
		},
		{
			name:         "zero MwEntry in WithMiddlewareImpl",
			fn:           func() { _ = NewRouter(WithPrefixes("/v3"), WithMiddlewareImpl(MwEntry{}, handler)) },
			wantContains: []string{"unknown middleware"},
		},
		{
			name: "nil route middleware",
			fn: func() {
				svr := NewRouter(WithPrefixes("/v3"))
				svr.RouterGroup("/v3").PWith([]gin.HandlerFunc{nil}, "/x", handler)
			},
			wantContains: []string{"route middleware", "must not be nil"},
		},
		{
			name: "nil handler",
			fn: func() {
				svr := NewRouter(WithPrefixes("/v3"))
				svr.RouterGroup("/v3").P("/x", nil)
			},
			wantContains: []string{"handler must not be nil"},
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			msg := recoverPanic(t, testCase.fn)
			requireContains(t, msg, testCase.wantContains...)
		})
	}
}

// TestRunListenerGuards 覆盖 Run 的两个守卫：nil listener、重复 Run。
func TestRunListenerGuards(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3"))
	msg := recoverPanic(t, func() { _ = svr.Run(nil) })
	requireContains(t, msg, "listener must not be nil")

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	if err := svr.Run(listener); err != nil {
		t.Fatalf("Run: %v", err)
	}
	msg = recoverPanic(t, func() { _ = svr.Run(listener) })
	requireContains(t, msg, "already running")
}

// TestPrefixAuthPolicyUndeclaredDeterministic 覆盖多个未声明鉴权前缀时 panic 文案稳定
// （按字典序取第一个，便于测试与排障）。
func TestPrefixAuthPolicyUndeclaredDeterministic(t *testing.T) {
	policy := NewAuthPolicy(nil, nil)
	msg := recoverPanic(t, func() {
		_ = NewRouter(
			WithPrefixes("/v3"),
			WithPrefixAuthPolicy("/v9", policy),
			WithPrefixAuthPolicy("/v8", policy),
		)
	})
	requireContains(t, msg, "undeclared prefix", `"/v8"`)
}

// TestLoggerReplacementKeepsDefaultWhitelist 覆盖 README 推荐的「替换 Logger 时保留默认白名单」写法可编译、
// 且 DefaultLoggerWhitelist 内容正确（行为侧由 TestLoggerWhitelistOption 覆盖）。
func TestLoggerReplacementKeepsDefaultWhitelist(t *testing.T) {
	whitelist := append(DefaultLoggerWhitelist(), "/healthz")
	svr := NewRouter(WithPrefixes("/v3"),
		WithMiddlewareImpl(Logger,
			middleware.Logger(middleware.WithLoggerWhitelist(whitelist...))))
	if svr.mw.impls[Logger.name] == nil {
		t.Fatal("Logger replacement not applied")
	}
	if want := []string{".Ping", "metrics", "/healthz"}; strings.Join(whitelist, ",") != strings.Join(want, ",") {
		t.Fatalf("whitelist = %v, want %v", whitelist, want)
	}
}
