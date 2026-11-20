package server

import (
	"net/http"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/apis/runtime/middleware"
)

// 本文件固化 v1.27「路由组投递」模型的对外契约：
// 注册入口唯一（RouterGroup / All()）、前缀精确集合且保序、广播必须显式、装配期 panic 语义、
// Routes() 四元组、文档与中间件按路由组生效。
// 断言只依赖公开 API 与同包可见的装配结果，全部离线执行。

// TestRegistrarInterface 覆盖验收「注册必须带路由组」：Router 自身不再是注册入口，
// RouterGroup 与 AllGroups 实现同一个 Registrar，模块路由函数只依赖该接口即可被投给任一前缀。
func TestRegistrarInterface(t *testing.T) {
	// 编译期断言：两个投递入口实现同一接口。
	var _ Registrar = (*RouterGroup)(nil)
	var _ Registrar = (*AllGroups)(nil)

	// 模拟下游模块函数：签名只收 Registrar，不知道自己被投给了哪个路由组。
	// 方法名与 1.23 线保持一致（含三个鉴权糖），迁移时模块函数体不用改。
	Routers := func(r Registrar) {
		r.P("/registrar.post", func(*gin.Context) {})
		r.G("/registrar.get", func(*gin.Context) {})
		r.Any("/registrar.any", func(*gin.Context) {})
		r.PWith(nil, "/registrar.pwith", func(*gin.Context) {})
		r.GWith([]gin.HandlerFunc{func(ctx *gin.Context) { ctx.Next() }}, "/registrar.gwith", func(*gin.Context) {})
		r.PRequireLogin("/registrar.plogin", func(*gin.Context) {})
		r.PRequireEmployee("/registrar.pemployee", func(*gin.Context) {})
		r.GRequireLogin("/registrar.glogin", func(*gin.Context) {})
		r.Docs("registrar")
	}

	t.Run("Router has no registration methods", func(t *testing.T) {
		svr := NewRouter(WithPrefixes("/v3"))
		// Router 不实现 Registrar：注册只能发生在 RouterGroup/AllGroups 上，不存在隐式广播入口。
		if _, ok := interface{}(svr).(Registrar); ok {
			t.Fatal("Router must not implement Registrar, registration entry must be RouterGroup/AllGroups only")
		}
	})

	t.Run("scope", func(t *testing.T) {
		svr := NewRouter(WithPrefixes("/v3", "/v2"))
		Routers(svr.RouterGroup("/v3"))
		routes := svr.Routes()
		if len(routes) != 10 {
			t.Fatalf("RouterGroup should register 10 routes (8 actions + 2 docs), got %d: %+v", len(routes), snapshotRoutes(svr))
		}
		for _, route := range routes {
			if route.Prefix != "/v3" {
				t.Fatalf("RouterGroup(/v3) leaked a route to prefix %q: %+v", route.Prefix, route)
			}
		}
	})

	t.Run("all", func(t *testing.T) {
		svr := NewRouter(WithPrefixes("/v3", "/v2"))
		Routers(svr.All())
		routes := svr.Routes()
		if len(routes) != 20 {
			t.Fatalf("All() over 2 prefixes should register 20 routes, got %d: %+v", len(routes), snapshotRoutes(svr))
		}
		counts := make(map[string]int)
		for _, route := range routes {
			counts[route.Prefix]++
		}
		if counts["/v3"] != 10 || counts["/v2"] != 10 {
			t.Fatalf("All() should broadcast 10 routes to each prefix, got %v", counts)
		}
	})
}

// TestPrefixesExact 覆盖验收「前缀精确集合」：声明什么就只有什么，
// 构造参数不再隐含前缀，且 "/v3"、"v3/" 归一化成同一个前缀。
func TestPrefixesExact(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3"))
	if want := []string{"/v3"}; !reflect.DeepEqual(svr.prefixes, want) {
		t.Fatalf("declared prefixes = %v, want %v", svr.prefixes, want)
	}
	if len(svr.ginGroups) != 1 {
		t.Fatalf("route groups = %d, want 1 (only /v3)", len(svr.ginGroups))
	}
	svr.RouterGroup("/v3").P("/user/login", func(*gin.Context) {})
	assertRouteSnapshot(t, svr, []routeKey{
		{Method: http.MethodPost, Path: "/v3/user/login", Prefix: "/v3"},
	})
	// gin 侧只有这一条，不存在任何隐含前缀带来的第二条路由。
	if set := ginRouteSet(svr); len(set) != 1 || !set["POST /v3/user/login"] {
		t.Fatalf("gin registered routes = %v, want only POST /v3/user/login", set)
	}

	// 归一化：无前导斜杠、带尾部斜杠都归一到 /v3，不会额外多出一个前缀。
	normalized := NewRouter(WithPrefixes("v3/"))
	if want := []string{"/v3"}; !reflect.DeepEqual(normalized.prefixes, want) {
		t.Fatalf("normalized prefixes = %v, want %v", normalized.prefixes, want)
	}
	normalized.RouterGroup("/v3/").G("/user/detail", func(*gin.Context) {})
	assertRouteSnapshot(t, normalized, []routeKey{
		{Method: http.MethodGet, Path: "/v3/user/detail", Prefix: "/v3"},
	})
}

// TestPrefixOrderStable 覆盖验收「前缀保序」：声明顺序 = prefixes 顺序 = 注册顺序，
// 且多次构造结果完全一致（旧实现用 map 遍历，启动顺序会漂）。
func TestPrefixOrderStable(t *testing.T) {
	want := []string{"/v3", "/v2", "/v1"}
	for i := 0; i < 5; i++ {
		svr := NewRouter(WithPrefixes("/v3", "/v2", "/v1"))
		if !reflect.DeepEqual(svr.prefixes, want) {
			t.Fatalf("round %d: prefixes = %v, want %v", i, svr.prefixes, want)
		}
	}

	svr := NewRouter(WithPrefixes("/v3", "/v2", "/v1"))
	svr.All().G("/user/detail", func(*gin.Context) {})
	routes := svr.Routes()
	if len(routes) != len(want) {
		t.Fatalf("broadcast registered %d routes, want %d", len(routes), len(want))
	}
	for i, prefix := range want {
		if routes[i].Prefix != prefix || routes[i].Path != prefix+"/user/detail" {
			t.Fatalf("registration order = %+v, want prefixes order %v", snapshotRoutes(svr), want)
		}
	}

	// 重复声明去重，并保留首次出现的位置。
	dedup := NewRouter(WithPrefixes("/v3", "/v3/", "/v2"))
	if wantDedup := []string{"/v3", "/v2"}; !reflect.DeepEqual(dedup.prefixes, wantDedup) {
		t.Fatalf("dedup prefixes = %v, want %v", dedup.prefixes, wantDedup)
	}
}

// TestNoImplicitPrefix 覆盖验收「不存在隐含默认前缀」：什么都不声明直接构造即 panic。
func TestNoImplicitPrefix(t *testing.T) {
	msg := recoverPanic(t, func() { NewRouter() })
	requireContains(t, msg, "no prefix declared")
}

// TestEmptyPrefixesPanics 覆盖失败语义：声明了空前缀集合等价于没有声明，构造期 panic。
func TestEmptyPrefixesPanics(t *testing.T) {
	msg := recoverPanic(t, func() { NewRouter(WithPrefixes()) })
	requireContains(t, msg, "no prefix declared")
}

// TestUndeclaredPrefixPanics 覆盖验收「未声明前缀早失败」：panic 消息给出可用前缀清单。
func TestUndeclaredPrefixPanics(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3", "/v2"))
	msg := recoverPanic(t, func() { svr.RouterGroup("/nope") })
	requireContains(t, msg, "undeclared prefix", "available prefixes", "/v3", "/v2")

	// 归一化之后依然匹配不上，同样早失败。
	msg = recoverPanic(t, func() { svr.RouterGroup(" nope/ ") })
	requireContains(t, msg, "undeclared prefix")
}

// TestAllBroadcast 覆盖验收「显式广播」：三前缀下 All().P 产生 3 条路由。
func TestAllBroadcast(t *testing.T) {
	prefixes := []string{"/v3", "/v4", "/v2"}
	svr := NewRouter(WithPrefixes(prefixes...))
	svr.All().P("/user/login", func(ctx *gin.Context) { ctx.String(http.StatusOK, "login-ok") })

	routes := svr.Routes()
	if len(routes) != len(prefixes) {
		t.Fatalf("All().P registered %d routes, want %d: %+v", len(routes), len(prefixes), snapshotRoutes(svr))
	}
	for i, prefix := range prefixes {
		if routes[i].Prefix != prefix || routes[i].Path != prefix+"/user/login" || routes[i].Method != http.MethodPost {
			t.Fatalf("route[%d] = %+v, want POST %s/user/login prefix=%s", i, routes[i], prefix, prefix)
		}
	}
	if set := ginRouteSet(svr); len(set) != len(prefixes) {
		t.Fatalf("gin registered %d routes, want %d: %v", len(set), len(prefixes), set)
	}
	// 每个前缀都真的能命中同一条业务 handler。
	for _, prefix := range prefixes {
		rec := doRequest(svr, http.MethodPost, prefix+"/user/login", nil, nil)
		if rec.Code != http.StatusOK || rec.Body.String() != "login-ok" {
			t.Fatalf("%s/user/login: status=%d body=%q, want 200 %q", prefix, rec.Code, rec.Body.String(), "login-ok")
		}
	}
}

// TestRouterGroupSinglePrefix 覆盖验收「单前缀投递」：RouterGroup("/v2").P 只产生 1 条，其余前缀 404。
func TestRouterGroupSinglePrefix(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3", "/v2", "/v4"))
	svr.RouterGroup("/v2").P("/user/login", func(ctx *gin.Context) { ctx.String(http.StatusOK, "v2-login") })

	assertRouteSnapshot(t, svr, []routeKey{
		{Method: http.MethodPost, Path: "/v2/user/login", Prefix: "/v2"},
	})
	if rec := doRequest(svr, http.MethodPost, "/v2/user/login", nil, nil); rec.Code != http.StatusOK || rec.Body.String() != "v2-login" {
		t.Fatalf("/v2/user/login: status=%d body=%q", rec.Code, rec.Body.String())
	}
	for _, prefix := range []string{"/v3", "/v4"} {
		rec := doRequest(svr, http.MethodPost, prefix+"/user/login", nil, nil)
		if rec.Code != http.StatusNotFound || rec.Body.String() != "The incorrect API route." {
			t.Fatalf("%s/user/login should be unregistered, got status=%d body=%q", prefix, rec.Code, rec.Body.String())
		}
	}
}

// sourcePattern 从 panic 文案里提取 "file:line" 注册来源。
var sourcePattern = regexp.MustCompile(`[^\s,"]+\.go:\d+`)

// TestDuplicateRoutePanics 覆盖验收「重复注册早失败」：消息含 path 与两个不同的注册来源。
func TestDuplicateRoutePanics(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3"))
	svr.RouterGroup("/v3").P("/user/login", func(*gin.Context) {})

	msg := recoverPanic(t, func() {
		svr.RouterGroup("/v3").P("/user/login", func(*gin.Context) {})
	})
	requireContains(t, msg, "duplicate route", "POST /v3/user/login")

	sources := sourcePattern.FindAllString(msg, -1)
	if len(sources) < 2 {
		t.Fatalf("duplicate panic must name two registration sources, got %v (message: %s)", sources, msg)
	}
	if sources[0] == sources[1] {
		t.Fatalf("duplicate panic sources must differ, got %v (message: %s)", sources, msg)
	}
	for _, source := range sources[:2] {
		if !strings.Contains(source, "_test.go:") {
			t.Fatalf("registration source %q should point into the test file", source)
		}
	}

	// 不同 method、同 path 不算重复：重复检测的键包含 method。
	svr2 := NewRouter(WithPrefixes("/v3"))
	svr2.RouterGroup("/v3").P("/user/login", func(*gin.Context) {})
	svr2.RouterGroup("/v3").G("/user/login", func(*gin.Context) {})
	assertRouteSnapshot(t, svr2, []routeKey{
		{Method: http.MethodGet, Path: "/v3/user/login", Prefix: "/v3"},
		{Method: http.MethodPost, Path: "/v3/user/login", Prefix: "/v3"},
	})
}

// TestRoutes 覆盖验收「清单结构化」：method/path/prefix/source 四元组，
// Path 与 gin 实际 fullPath 一致，且 Routes() 返回副本。
func TestRoutes(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3", "/v2"))
	scope := svr.RouterGroup("/v3")
	scope.G("/user/detail", func(*gin.Context) {})
	scope.P("/user/login", func(*gin.Context) {})

	routes := svr.Routes()
	if len(routes) != 2 {
		t.Fatalf("Routes() returned %d entries, want 2", len(routes))
	}
	// 注册顺序：先 G 后 P。
	if routes[0].Method != http.MethodGet || routes[0].Path != "/v3/user/detail" {
		t.Fatalf("routes[0] = %+v, want GET /v3/user/detail", routes[0])
	}
	if routes[1].Method != http.MethodPost || routes[1].Path != "/v3/user/login" {
		t.Fatalf("routes[1] = %+v, want POST /v3/user/login", routes[1])
	}
	ginSet := ginRouteSet(svr)
	for _, route := range routes {
		if route.Prefix != "/v3" {
			t.Fatalf("route prefix = %q, want /v3: %+v", route.Prefix, route)
		}
		if !strings.Contains(route.Source, "_test.go:") {
			t.Fatalf("route source = %q, want test file:line", route.Source)
		}
		if !ginSet[route.Method+" "+route.Path] {
			t.Fatalf("Routes().Path %q does not match any gin fullPath: %v", route.Path, ginSet)
		}
	}

	// 副本语义：改返回值 / 追加都不影响 Router 内部清单。
	routes[0].Path = "/mutated"
	routes = append(routes, RouteInfo{Method: http.MethodGet, Path: "/injected"})
	after := svr.Routes()
	if len(after) != 2 {
		t.Fatalf("Router route list was mutated by caller, len = %d, want 2", len(after))
	}
	if after[0].Path != "/v3/user/detail" {
		t.Fatalf("Router route list was mutated by caller: %+v", after[0])
	}
}

// TestDocsPerRouterGroup 覆盖验收「文档按路由组」：RouterGroup.Docs 只挂该前缀；All().Docs 每个前缀各 2 条。
func TestDocsPerRouterGroup(t *testing.T) {
	docsRoutes := func(prefix, model string) []routeKey {
		return []routeKey{
			{Method: http.MethodGet, Path: prefix + "/" + model + ".docs/*any", Prefix: prefix},
			{Method: http.MethodGet, Path: prefix + "/" + model + ".redocs", Prefix: prefix},
		}
	}

	single := NewRouter(WithPrefixes("/v3", "/v2"))
	single.RouterGroup("/v3").Docs("app")
	assertRouteSnapshot(t, single, docsRoutes("/v3", "app"))
	// 未挂载的前缀没有文档路由，访问 404。
	if rec := doRequest(single, http.MethodGet, "/v2/app.redocs", nil, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("/v2/app.redocs should be 404, got %d", rec.Code)
	}

	all := NewRouter(WithPrefixes("/v3", "/v2"))
	all.All().Docs("app")
	want := append(docsRoutes("/v3", "app"), docsRoutes("/v2", "app")...)
	assertRouteSnapshot(t, all, want)
}

// TestPrefixMiddleware 覆盖验收「前缀级中间件」：只有该前缀的响应带标记头。
func TestPrefixMiddleware(t *testing.T) {
	marker := func(value string) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			ctx.Header("X-Prefix-Middleware", value)
			ctx.Next()
		}
	}
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "who") }

	svr := NewRouter(WithPrefixes("/v3"), WithPrefix("/v2", marker("v2")))
	if want := []string{"/v3", "/v2"}; !reflect.DeepEqual(svr.prefixes, want) {
		t.Fatalf("prefixes = %v, want %v", svr.prefixes, want)
	}
	svr.RouterGroup("/v3").G("/who", handler)
	svr.RouterGroup("/v2").G("/who", handler)
	svr.RouterGroup("/v2").G("/other", handler)

	for _, path := range []string{"/v2/who", "/v2/other"} {
		rec := doRequest(svr, http.MethodGet, path, nil, nil)
		if rec.Code != http.StatusOK || rec.Body.String() != "who" {
			t.Fatalf("%s: status=%d body=%q, want 200/who", path, rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Prefix-Middleware"); got != "v2" {
			t.Fatalf("%s should carry prefix middleware header, got %q", path, got)
		}
	}
	rec := doRequest(svr, http.MethodGet, "/v3/who", nil, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "who" {
		t.Fatalf("/v3/who: status=%d body=%q, want 200/who", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Prefix-Middleware"); got != "" {
		t.Fatalf("/v3/who must not carry /v2 prefix middleware, got header %q", got)
	}
}

// TestRouteMiddleware 覆盖验收「路由级中间件」：PWith 只作用于该路由，且顺序在 handler 之前。
func TestRouteMiddleware(t *testing.T) {
	routeMW := func(value string) gin.HandlerFunc {
		return func(ctx *gin.Context) {
			ctx.Writer.Header().Add("X-Route-Middleware", value)
			ctx.Next()
		}
	}
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "route-ok") }

	svr := NewRouter(WithPrefixes("/v3"))
	svr.RouterGroup("/v3").PWith([]gin.HandlerFunc{routeMW("1"), routeMW("2")}, "/user/logout", handler)
	svr.RouterGroup("/v3").P("/user/login", handler)
	assertRouteSnapshot(t, svr, []routeKey{
		{Method: http.MethodPost, Path: "/v3/user/login", Prefix: "/v3"},
		{Method: http.MethodPost, Path: "/v3/user/logout", Prefix: "/v3"},
	})

	withMW := doRequest(svr, http.MethodPost, "/v3/user/logout", nil, nil)
	if withMW.Code != http.StatusOK || withMW.Body.String() != "route-ok" {
		t.Fatalf("/v3/user/logout: status=%d body=%q", withMW.Code, withMW.Body.String())
	}
	if got := withMW.Header().Values("X-Route-Middleware"); !reflect.DeepEqual(got, []string{"1", "2"}) {
		t.Fatalf("route middleware chain = %v, want [1 2] in declaration order", got)
	}

	withoutMW := doRequest(svr, http.MethodPost, "/v3/user/login", nil, nil)
	if got := withoutMW.Header().Values("X-Route-Middleware"); len(got) != 0 {
		t.Fatalf("/v3/user/login must not run route-level middleware, got %v", got)
	}
}

// TestAuthSugar 覆盖「保留 1.23 风格鉴权糖」：
// PRequireLogin / PRequireEmployee / GRequireLogin 必须与
// PWith/GWith + AuthMiddleWare/AuthMiddleWareEmployee 完全等价（Routes() 快照 + 实际行为），
// 这样既有模块路由函数迁移时调用风格保持不变；AllGroups 上按声明顺序广播。
func TestAuthSugar(t *testing.T) {
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "sugar-ok") }
	// API Key 分支让 LoginStatus 进入 StateSucc、Role=RoleAPI，无需 JWT/settings。
	apiKeyAuth := map[string]string{"Authorization": auth.AuthBearer + auth.AuthAPIKeyPrefix + "test"}

	sugar := NewRouter(WithPrefixes("/v3"))
	sugar.RouterGroup("/v3").PRequireLogin("/user.profile", handler)
	sugar.RouterGroup("/v3").PRequireEmployee("/user.employee", handler)
	sugar.RouterGroup("/v3").GRequireLogin("/user.session", handler)

	explicit := NewRouter(WithPrefixes("/v3"))
	explicit.RouterGroup("/v3").PWith([]gin.HandlerFunc{middleware.AuthMiddleWare}, "/user.profile", handler)
	explicit.RouterGroup("/v3").PWith([]gin.HandlerFunc{middleware.AuthMiddleWareEmployee}, "/user.employee", handler)
	explicit.RouterGroup("/v3").GWith([]gin.HandlerFunc{middleware.AuthMiddleWare}, "/user.session", handler)

	if got, want := snapshotRoutes(sugar), snapshotRoutes(explicit); !reflect.DeepEqual(got, want) {
		t.Fatalf("sugar Routes() = %+v, want explicit PWith/GWith = %+v", got, want)
	}

	unauthorized := `"code":401`
	anonCases := []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/v3/user.profile"},
		{http.MethodPost, "/v3/user.employee"},
		{http.MethodGet, "/v3/user.session"},
	}
	for _, testCase := range anonCases {
		rec := doRequest(sugar, testCase.method, testCase.path, nil, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), unauthorized) {
			t.Fatalf("anonymous %s %s: status=%d body=%q, want 200 with code 401", testCase.method, testCase.path, rec.Code, rec.Body.String())
		}
	}

	// API Key 已登录（RoleAPI）：PRequireLogin / GRequireLogin 放行，PRequireEmployee 仍拒绝。
	rec := doRequest(sugar, http.MethodPost, "/v3/user.profile", apiKeyAuth, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "sugar-ok" {
		t.Fatalf("PRequireLogin with api-key: status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = doRequest(sugar, http.MethodGet, "/v3/user.session", apiKeyAuth, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "sugar-ok" {
		t.Fatalf("GRequireLogin with api-key: status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = doRequest(sugar, http.MethodPost, "/v3/user.employee", apiKeyAuth, nil)
	if !strings.Contains(rec.Body.String(), unauthorized) {
		t.Fatalf("PRequireEmployee must reject RoleAPI principal, got status=%d body=%q", rec.Code, rec.Body.String())
	}

	// AllGroups 上的三个糖：每次注册按声明顺序遍历全部前缀（一条路由先投满所有前缀，再投下一条）。
	all := NewRouter(WithPrefixes("/v3", "/v4"))
	all.All().PRequireLogin("/user.profile", handler)
	all.All().PRequireEmployee("/user.employee", handler)
	all.All().GRequireLogin("/user.session", handler)
	routes := all.Routes()
	wantOrder := []RouteInfo{
		{Method: http.MethodPost, Path: "/v3/user.profile", Prefix: "/v3"},
		{Method: http.MethodPost, Path: "/v4/user.profile", Prefix: "/v4"},
		{Method: http.MethodPost, Path: "/v3/user.employee", Prefix: "/v3"},
		{Method: http.MethodPost, Path: "/v4/user.employee", Prefix: "/v4"},
		{Method: http.MethodGet, Path: "/v3/user.session", Prefix: "/v3"},
		{Method: http.MethodGet, Path: "/v4/user.session", Prefix: "/v4"},
	}
	if len(routes) != len(wantOrder) {
		t.Fatalf("AllGroups auth sugar registered %d routes, want %d", len(routes), len(wantOrder))
	}
	for i, want := range wantOrder {
		got := routes[i]
		if got.Method != want.Method || got.Path != want.Path || got.Prefix != want.Prefix {
			t.Fatalf("route[%d] = %+v, want %+v (per action, then declaration order)", i, got, want)
		}
	}
}

// TestSealedOptionsPanic 覆盖失败语义「声明必须在构造期完成」：NewRouter 之后再用 option 一律 panic。
func TestSealedOptionsPanic(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3"))
	sealed := WithPrefixes("/x")
	requireContains(t, recoverPanic(t, func() { sealed(svr) }), "sealed")

	options := []struct {
		name string
		opt  RouterOption
	}{
		{"WithPrefixes", WithPrefixes("/x")},
		{"WithPrefix", WithPrefix("/x")},
		{"WithPrefixSpecs", WithPrefixSpecs(PrefixSpec{Prefix: "/x"})},
		{"WithMiddleware", WithMiddleware(func(*gin.Context) {})},
		{"WithoutMiddleware", WithoutMiddleware(CORS)},
		{"WithMiddlewareImpl", WithMiddlewareImpl(Recovery, func(*gin.Context) {})},
		{"WithMiddlewareBefore", WithMiddlewareBefore(Language, func(*gin.Context) {})},
	}
	for _, option := range options {
		t.Run(option.name, func(t *testing.T) {
			requireContains(t, recoverPanic(t, func() { option.opt(svr) }), "sealed")
		})
	}
}
