package server

import (
	"bytes"
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/apiobj"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/errcode"
)

// 本文件是 P6「API 映射表」的可执行版本：
//   - TestMappingTable：把设计文档「破坏性变更清单与迁移映射」里 1.23 线的五种注册形态
//     （单前缀、多前缀广播、GinEngine 探针、hdrs 里塞中间件、按前缀挂文档）
//     各写一份 1.27 等价写法，断言 Routes() 快照与旧语义一致。
//   - TestProbeNotInRoutes：探针属于逃生口，不进 Routes() 索引。
//   - TestHandlerEquivalence：四种 handler 签名 + BaseResponse 修正逐字节对拍，
//     golden 由当前实现跑出来再固化（handler.go 本阶段不动，跑出来的就是等价基线）。

// testAPIIn / testAPIOut 是最小业务结构，用来验证 transAPI 的反射适配与响应修正。
type testAPIIn struct {
	Value string `json:"value"`
}

type testAPIData struct {
	Value string `json:"value"`
}

type testAPIOut struct {
	apiobj.BaseResponse
	Data *testAPIData `json:"data,omitempty"`
}

// testHTTPHandler 覆盖 http.Handler 签名。
type testHTTPHandler struct{}

// ServeHTTP 走 transHttpHdr 适配路径。
func (testHTTPHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusAccepted)
	_, _ = w.Write([]byte("handler-ok"))
}

// TestMappingTable 是 P6 的表驱动对拍：同一语义用新 API 写一遍，
// 断言 method/path/prefix 快照（排序后）等于 1.23 线语义应产生的集合。
func TestMappingTable(t *testing.T) {
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") }
	gated := func(ctx *gin.Context) {
		ctx.Header("X-Gated", "1")
		ctx.Next()
	}
	// gatedCheck 断言 hdrs/PWith 两种写法的中间件都真的在 handler 之前执行。
	gatedCheck := func(t *testing.T, svr *Router) {
		t.Helper()
		rec := doRequest(svr, http.MethodPost, "/v3/auth.Logout", nil, nil)
		if rec.Code != http.StatusOK || rec.Body.String() != "ok" {
			t.Fatalf("/v3/auth.Logout: status=%d body=%q", rec.Code, rec.Body.String())
		}
		if got := rec.Header().Get("X-Gated"); got != "1" {
			t.Fatalf("gated middleware did not run before handler, header = %q", got)
		}
	}

	cases := []struct {
		name  string
		build func(t *testing.T) *Router
		want  []routeKey
		check func(t *testing.T, svr *Router)
	}{
		{
			name: `single prefix: 旧 NewRouter("/v3") + svr.P("/user/login", h)`,
			build: func(t *testing.T) *Router {
				t.Helper()
				svr := NewRouter(WithPrefixes("/v3"))
				svr.RouterGroup("/v3").P("/user/login", handler)
				return svr
			},
			want: []routeKey{
				{Method: http.MethodPost, Path: "/v3/user/login", Prefix: "/v3"},
			},
		},
		{
			name: `multi prefix broadcast: 旧 WithPrefixes(["/v3/","/v4/"]) + svr.P 产生 2 条`,
			build: func(t *testing.T) *Router {
				t.Helper()
				svr := NewRouter(WithPrefixes("/v3", "/v4"))
				svr.All().P("/user/login", handler)
				return svr
			},
			want: []routeKey{
				{Method: http.MethodPost, Path: "/v3/user/login", Prefix: "/v3"},
				{Method: http.MethodPost, Path: "/v4/user/login", Prefix: "/v4"},
			},
		},
		{
			name: `probe: 旧 svr.GinEngine().GET("/healthz", h) 不进 Routes()`,
			build: func(t *testing.T) *Router {
				t.Helper()
				svr := NewRouter(WithPrefixes("/v3"))
				svr.RouterGroup("/v3").P("/user/login", handler)
				svr.GinEngine().GET("/healthz", func(ctx *gin.Context) { ctx.String(http.StatusOK, "probe-ok") })
				return svr
			},
			want: []routeKey{
				{Method: http.MethodPost, Path: "/v3/user/login", Prefix: "/v3"},
			},
			check: func(t *testing.T, svr *Router) {
				t.Helper()
				rec := doRequest(svr, http.MethodGet, "/healthz", nil, nil)
				if rec.Code != http.StatusOK || rec.Body.String() != "probe-ok" {
					t.Fatalf("/healthz probe: status=%d body=%q", rec.Code, rec.Body.String())
				}
			},
		},
		{
			name: `hdrs 里塞中间件: scope.P("auth.Logout", gated, action)`,
			build: func(t *testing.T) *Router {
				t.Helper()
				svr := NewRouter(WithPrefixes("/v3"))
				svr.RouterGroup("/v3").P("auth.Logout", gated, handler)
				return svr
			},
			want: []routeKey{
				{Method: http.MethodPost, Path: "/v3/auth.Logout", Prefix: "/v3"},
			},
			check: gatedCheck,
		},
		{
			name: `hdrs 显式版本: scope.PWith([]gin.HandlerFunc{gated}, "auth.Logout", action)`,
			build: func(t *testing.T) *Router {
				t.Helper()
				svr := NewRouter(WithPrefixes("/v3"))
				svr.RouterGroup("/v3").PWith([]gin.HandlerFunc{gated}, "auth.Logout", handler)
				return svr
			},
			want: []routeKey{
				{Method: http.MethodPost, Path: "/v3/auth.Logout", Prefix: "/v3"},
			},
			check: gatedCheck,
		},
		{
			name: `auth sugar: 旧 svr.PRequireLogin(...) → scope.PRequireLogin(...)（同名同语义保留）`,
			build: func(t *testing.T) *Router {
				t.Helper()
				svr := NewRouter(WithPrefixes("/v3"))
				svr.RouterGroup("/v3").PRequireLogin("user.Profile", handler)
				return svr
			},
			want: []routeKey{
				{Method: http.MethodPost, Path: "/v3/user.Profile", Prefix: "/v3"},
			},
			check: func(t *testing.T, svr *Router) {
				t.Helper()
				// 未登录：鉴权中间件拦下，body 里 code 401（HTTP 仍 200，与 1.23 线一致）。
				rec := doRequest(svr, http.MethodPost, "/v3/user.Profile", nil, nil)
				if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code":401`) {
					t.Fatalf("/v3/user.Profile: status=%d body=%q, want 200 with code 401", rec.Code, rec.Body.String())
				}
			},
		},
		{
			name: `docs: 旧 svr.HandleDoc("app") 铺满前缀 → svr.All().Docs("app")`,
			build: func(t *testing.T) *Router {
				t.Helper()
				svr := NewRouter(WithPrefixes("/v3", "/v4"))
				svr.All().Docs("app")
				return svr
			},
			want: []routeKey{
				{Method: http.MethodGet, Path: "/v3/app.docs/*any", Prefix: "/v3"},
				{Method: http.MethodGet, Path: "/v3/app.redocs", Prefix: "/v3"},
				{Method: http.MethodGet, Path: "/v4/app.docs/*any", Prefix: "/v4"},
				{Method: http.MethodGet, Path: "/v4/app.redocs", Prefix: "/v4"},
			},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			svr := testCase.build(t)
			assertRouteSnapshot(t, svr, testCase.want)
			if testCase.check != nil {
				testCase.check(t, svr)
			}
		})
	}

	// 两种 hdrs 写法（隐式塞中间件 / 显式 PWith）必须完全等价。
	t.Run("hdrs middleware and PWith are equivalent", func(t *testing.T) {
		implicit := NewRouter(WithPrefixes("/v3"))
		implicit.RouterGroup("/v3").P("auth.Logout", gated, handler)
		explicit := NewRouter(WithPrefixes("/v3"))
		explicit.RouterGroup("/v3").PWith([]gin.HandlerFunc{gated}, "auth.Logout", handler)

		if got, want := snapshotRoutes(explicit), snapshotRoutes(implicit); len(got) != 1 || len(want) != 1 || got[0] != want[0] {
			t.Fatalf("Routes() differ: implicit=%+v explicit=%+v", want, got)
		}
		gatedCheck(t, implicit)
		gatedCheck(t, explicit)
	})
}

// TestProbeNotInRoutes 覆盖验收「探针走逃生口」：GinEngine() 直挂的探针不进 Routes()，
// 也不参与重复检测，但请求仍然可达；之后注册的业务路由照常进清单。
func TestProbeNotInRoutes(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3"))
	svr.GinEngine().GET("/healthz", func(ctx *gin.Context) { ctx.String(http.StatusOK, "probe-ok") })

	if got := svr.Routes(); len(got) != 0 {
		t.Fatalf("probe should not enter Routes(), got %+v", got)
	}
	if !ginRouteSet(svr)["GET /healthz"] {
		t.Fatalf("probe should still be registered on gin engine")
	}
	if rec := doRequest(svr, http.MethodGet, "/healthz", nil, nil); rec.Code != http.StatusOK || rec.Body.String() != "probe-ok" {
		t.Fatalf("/healthz probe: status=%d body=%q", rec.Code, rec.Body.String())
	}

	// 探针不进索引，因此不与业务路由的重复检测互相干扰。
	svr.RouterGroup("/v3").G("/user/detail", func(ctx *gin.Context) { ctx.String(http.StatusOK, "detail") })
	if got := snapshotRoutes(svr); len(got) != 1 || got[0] != (routeKey{Method: http.MethodGet, Path: "/v3/user/detail", Prefix: "/v3"}) {
		t.Fatalf("Routes() = %+v, want only GET /v3/user/detail", got)
	}
}

// responseGolden 是逐字节对拍的期望响应。
type responseGolden struct {
	status int
	body   string
}

// TestHandlerEquivalence 覆盖验收「handler 等价」：四种 handler 签名 + BaseResponse 修正
// 的响应状态码与响应体字节与 1.27 重构前完全一致（golden 由当前 handler.go 实跑捕获）。
func TestHandlerEquivalence(t *testing.T) {
	// 反射签名：func(*gin.Context, *In, *Out) error，Out 内嵌 BaseResponse 验证修正逻辑。
	apiEcho := func(ctx *gin.Context, in *testAPIIn, out *testAPIOut) error {
		out.Code = 0
		out.Data = &testAPIData{Value: "api:" + in.Value}
		return nil
	}
	// translateMessage：Message 非空时按 Accept-Language 翻译；未初始化 i18n 时原样返回。
	apiMessage := func(ctx *gin.Context, in *testAPIIn, out *testAPIOut) error {
		out.Message = "hello"
		out.Data = &testAPIData{Value: "message"}
		return nil
	}
	// fixBaseResponse：Message 为空时用 errcode 填 code 对应文案。
	apiErrCode := func(ctx *gin.Context, in *testAPIIn, out *testAPIOut) error {
		out.Code = errcode.ErrCode_WrongUsernameOrPassword
		out.Data = &testAPIData{Value: "errcode"}
		return nil
	}
	// fixBaseResponse：RequestID 取 context 里的 reqid。
	apiRequestID := func(ctx *gin.Context, in *testAPIIn, out *testAPIOut) error {
		ctx.Set(constants.CtxKeyRequestID, "reqid-golden")
		out.Data = &testAPIData{Value: "reqid"}
		return nil
	}
	ginHandler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "gin-ok") }
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte("http-ok"))
	})

	svr := NewRouter(WithPrefixes("/v3"))
	scope := svr.RouterGroup("/v3")
	scope.P("/api.echo", apiEcho)
	scope.P("/api.message", apiMessage)
	scope.P("/api.errcode", apiErrCode)
	scope.P("/api.reqid", apiRequestID)
	scope.P("/gin.echo", ginHandler)
	scope.P("/http.echo", httpHandler)
	scope.P("/handler.echo", testHTTPHandler{})
	if got := len(svr.Routes()); got != 7 {
		t.Fatalf("registered %d routes, want 7", got)
	}

	cases := []struct {
		name string
		path string
		body string
		want responseGolden
	}{
		{
			name: "reflection api handler",
			path: "/v3/api.echo",
			body: `{"value":"input"}`,
			want: responseGolden{status: http.StatusOK, body: `{"code":0,"data":{"value":"api:input"}}`},
		},
		{
			name: "base response message",
			path: "/v3/api.message",
			body: `{}`,
			want: responseGolden{status: http.StatusOK, body: `{"code":0,"message":"hello","data":{"value":"message"}}`},
		},
		{
			name: "base response errcode message",
			path: "/v3/api.errcode",
			body: `{}`,
			want: responseGolden{status: http.StatusOK, body: `{"code":10001,"message":"用户名或密码错误","data":{"value":"errcode"}}`},
		},
		{
			name: "base response request_id",
			path: "/v3/api.reqid",
			body: `{}`,
			want: responseGolden{status: http.StatusOK, body: `{"code":0,"request_id":"reqid-golden","data":{"value":"reqid"}}`},
		},
		{
			name: "gin handler",
			path: "/v3/gin.echo",
			want: responseGolden{status: http.StatusOK, body: "gin-ok"},
		},
		{
			name: "http handler func",
			path: "/v3/http.echo",
			want: responseGolden{status: http.StatusCreated, body: "http-ok"},
		},
		{
			name: "http handler",
			path: "/v3/handler.echo",
			want: responseGolden{status: http.StatusAccepted, body: "handler-ok"},
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			rec := doRequest(svr, http.MethodPost, testCase.path, nil, bytes.NewBufferString(testCase.body))
			if rec.Code != testCase.want.status {
				t.Fatalf("%s status = %d, want %d (body %q)", testCase.path, rec.Code, testCase.want.status, rec.Body.String())
			}
			if got := rec.Body.String(); got != testCase.want.body {
				t.Fatalf("%s body = %q, want %q", testCase.path, got, testCase.want.body)
			}
		})
	}
}
