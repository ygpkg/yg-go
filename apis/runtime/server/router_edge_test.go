package server

import (
	"net"
	"net/http"
	"reflect"
	"testing"

	"github.com/gin-gonic/gin"
)

// 本文件补齐评审指出的边界契约：Router 方法集黑名单、Any 跨方法重复检测、
// 路径矩阵与 gin fullPath 等价、前缀 path.Clean、三层中间件顺序、Run 后注册守卫、
// WithPrefixGroups 正向语义。

// TestRegistrarHasNoRegistrationMethods 逐方法断言 Router 上不存在注册入口与旧符号。
//
// 只断言「Router 不实现 Registrar」不够：加回单个 P/G 时接口断言仍会通过。
func TestRegistrarHasNoRegistrationMethods(t *testing.T) {
	routerType := reflect.TypeOf((*Router)(nil))
	for _, method := range []string{
		"P", "G", "Any", "PWith", "GWith",
		"PRequireLogin", "PRequireEmployee", "GRequireLogin",
		"Docs", "HandleDoc", "ListAllRouters",
	} {
		if _, ok := routerType.MethodByName(method); ok {
			t.Fatalf("Router must not expose registration method %q", method)
		}
	}
	if _, ok := routerType.Elem().FieldByName("Prefix"); ok {
		t.Fatal("Router must not expose Prefix field")
	}
	// 反向保护：这些入口必须还在（防止误删）。
	for _, method := range []string{"Scope", "All", "Routes", "LogRoutes", "GinEngine", "Run"} {
		if _, ok := routerType.MethodByName(method); !ok {
			t.Fatalf("Router lost required method %q", method)
		}
	}
}

// TestAnyCrossMethodDuplicatePanics 覆盖 Any 与 P/G 混用同 path 的启动期检测：
// gin 自身只会报 handlers are already registered（无来源），这里必须给出 server 的重复注册来源。
func TestAnyCrossMethodDuplicatePanics(t *testing.T) {
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") }
	cases := []struct {
		name   string
		first  func(Registrar)
		second func(Registrar)
		key    string
	}{
		{
			name:   "P then Any",
			first:  func(registrar Registrar) { registrar.P("/x", handler) },
			second: func(registrar Registrar) { registrar.Any("/x", handler) },
			key:    "POST /v3/x",
		},
		{
			name:   "Any then G",
			first:  func(registrar Registrar) { registrar.Any("/x", handler) },
			second: func(registrar Registrar) { registrar.G("/x", handler) },
			key:    "GET /v3/x",
		},
		{
			name:   "Any then Any",
			first:  func(registrar Registrar) { registrar.Any("/x", handler) },
			second: func(registrar Registrar) { registrar.Any("/x", handler) },
			key:    "GET /v3/x",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			svr := NewRouter(WithPrefixes("/v3"))
			scope := svr.Scope("/v3")
			testCase.first(scope)
			msg := recoverPanic(t, func() { testCase.second(scope) })
			requireContains(t, msg, "duplicate route", testCase.key)
		})
	}
}

// TestAnyDispatchesAllMethods 验证 Any 注册的路由对多个方法都可达（而不只是能注册成功）。
func TestAnyDispatchesAllMethods(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3"))
	svr.Scope("/v3").Any("/any.probe", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete, http.MethodPatch} {
		rec := doRequest(svr, method, "/v3/any.probe", nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s /v3/any.probe: status=%d, want 200", method, rec.Code)
		}
	}
}

// TestPathMatrixMatchesGin 钉住重复检测键的前提：Routes().Path 与 gin 实际 fullPath 逐字一致。
func TestPathMatrixMatchesGin(t *testing.T) {
	actions := []string{
		"/user/login", "user/login", "/user/login/", "/", "",
		"app.docs/*any", "user/:id", "/user/:id/",
	}
	for _, action := range actions {
		svr := NewRouter(WithPrefixes("/v3"))
		svr.Scope("/v3").P(action, func(ctx *gin.Context) {})
		routes := svr.Routes()
		if len(routes) != 1 {
			t.Fatalf("action %q: Routes() len=%d", action, len(routes))
		}
		var ginPath string
		for _, route := range svr.GinEngine().Routes() {
			if route.Method == http.MethodPost {
				ginPath = route.Path
			}
		}
		if routes[0].Path != ginPath {
			t.Fatalf("action %q: Routes().Path=%q, gin fullPath=%q", action, routes[0].Path, ginPath)
		}
	}
}

// TestNormalizePrefixClean 覆盖非规范前缀：`//` 与尾斜杠归一，`.`/`..` 段直接拒绝，
// 保证 Scope 查表与真实 fullPath 不分叉，也不会把拼错的前缀静默折叠成根前缀。
func TestNormalizePrefixClean(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3//x", "/v3/", "//"))
	if want := []string{"/v3/x", "/v3", "/"}; !reflect.DeepEqual(svr.prefixes, want) {
		t.Fatalf("prefixes = %v, want %v", svr.prefixes, want)
	}
	svr.Scope("/v3/x").P("/clean.probe", func(ctx *gin.Context) {})
	if got := svr.Routes()[0].Prefix; got != "/v3/x" {
		t.Fatalf("route prefix = %q, want /v3/x", got)
	}
	if rec := doRequest(svr, http.MethodPost, "/v3/x/clean.probe", nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("cleaned prefix route status = %d, want 200", rec.Code)
	}
	// 根前缀可正常注册与访问。
	svr.Scope("/").G("/root.probe", func(ctx *gin.Context) { ctx.String(http.StatusOK, "root") })
	if rec := doRequest(svr, http.MethodGet, "/root.probe", nil, nil); rec.Code != http.StatusOK {
		t.Fatalf("root prefix route status = %d, want 200", rec.Code)
	}
	// 重复声明去重也走同一口径。
	dedup := NewRouter(WithPrefixes("/v3//x", "/v3/x", "v3/x/"))
	if want := []string{"/v3/x"}; !reflect.DeepEqual(dedup.prefixes, want) {
		t.Fatalf("dedup prefixes = %v, want %v", dedup.prefixes, want)
	}
	// `.` / `..` 段是拼写错误信号，拒绝而不是折叠成根前缀。
	for _, bad := range []string{"/v3/../v4", "..", "/v3/.", "."} {
		msg := recoverPanic(t, func() { _ = NewRouter(WithPrefixes(bad)) })
		requireContains(t, msg, "must not contain", "segment")
	}
}

// TestThreeLayerMiddlewareOrder 验证文档承诺的引擎链 → 前缀级 → 路由级 → handler 顺序。
func TestThreeLayerMiddlewareOrder(t *testing.T) {
	appendOrder := func(layer string) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			order := ctx.Writer.Header().Get("X-Order")
			if order == "" {
				ctx.Header("X-Order", layer)
			} else {
				ctx.Header("X-Order", order+","+layer)
			}
			ctx.Next()
		}
	}
	svr := NewRouter(
		WithPrefixes("/v3"),
		WithMiddleware(appendOrder("engine")),
		WithPrefixGroup("/v3", appendOrder("prefix")),
	)
	svr.Scope("/v3").PWith([]gin.HandlerFunc{appendOrder("route")}, "/order.probe",
		func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })
	rec := doRequest(svr, http.MethodPost, "/v3/order.probe", nil, nil)
	if got := rec.Header().Get("X-Order"); got != "engine,prefix,route" {
		t.Fatalf("X-Order = %q, want engine,prefix,route", got)
	}
}

// TestRunAfterRegistrationPanics 覆盖 Run 之后继续注册：启动期 panic，而不是并发读写 gin 树。
func TestRunAfterRegistrationPanics(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3"))
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := svr.Run(listener); err != nil {
		t.Fatalf("Run: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	msg := recoverPanic(t, func() {
		svr.Scope("/v3").P("/late.register", func(ctx *gin.Context) {})
	})
	requireContains(t, msg, "before Run")
}

// TestWithPrefixGroupsPositive 覆盖批量声明的正向语义：保序 + 各前缀中间件只对自己生效。
func TestWithPrefixGroupsPositive(t *testing.T) {
	marker := func(name string) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			ctx.Header("X-Prefix-Group", name)
			ctx.Next()
		}
	}
	svr := NewRouter(WithPrefixGroups(
		PrefixGroup{Prefix: "/v2", Middlewares: []gin.HandlerFunc{marker("v2")}},
		PrefixGroup{Prefix: "/v3/"},
	))
	if want := []string{"/v2", "/v3"}; !reflect.DeepEqual(svr.prefixes, want) {
		t.Fatalf("prefixes = %v, want %v", svr.prefixes, want)
	}
	svr.Scope("/v2").G("/group.probe", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })
	svr.Scope("/v3").G("/group.probe", func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") })

	if got := doRequest(svr, http.MethodGet, "/v2/group.probe", nil, nil).Header().Get("X-Prefix-Group"); got != "v2" {
		t.Fatalf("/v2 marker = %q, want v2", got)
	}
	if got := doRequest(svr, http.MethodGet, "/v3/group.probe", nil, nil).Header().Get("X-Prefix-Group"); got != "" {
		t.Fatalf("/v3 should not inherit v2 middleware, got %q", got)
	}
}
