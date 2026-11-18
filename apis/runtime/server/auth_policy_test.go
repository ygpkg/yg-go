package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 本文件固化「账号体系可替换」的契约：
//   - PRequireLogin / GRequireLogin 使用 AuthPolicy.Login()，PRequireEmployee 使用 AuthPolicy.Employee()；
//   - 默认策略与 1.23 线行为一致（读 runtime.LoginStatus）；
//   - 自定义策略可以完全绕开 LoginStatus（配合 WithoutMiddleware(LoginStatus)，不会再 MustGet panic），
//     并且由策略自己决定拒绝响应的形态（不同服务的错误码/文案不同）；
//   - 同一个 Router 内可以按前缀覆盖策略（多版本不同账号体系）。

// requireHeaderGate 是「身份闸」的最小示例：请求头存在即放行，否则写自己的错误信封并 Abort。
func requireHeaderGate(header, code, message string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		if ctx.GetHeader(header) == "" {
			ctx.AbortWithStatusJSON(http.StatusOK, gin.H{"code": code, "message": message})
			return
		}
		ctx.Next()
	}
}

// TestAuthPolicyCustomGuard 覆盖：账号体系不经过 yg-go LoginStatus 的服务，
// 用自定义 AuthPolicy 后 PRequireLogin 等三个糖照常可用，且不再依赖 LoginStatus。
func TestAuthPolicyCustomGuard(t *testing.T) {
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "policy-ok") }
	loginGate := requireHeaderGate("X-Login-Gate", "login-gate-denied", "log in via canvas first")
	employeeGate := requireHeaderGate("X-Employee-Gate", "employee-gate-denied", "not an employee")

	svr := NewRouter(
		WithPrefixes("/api/v1"),
		// 账号体系不走 yg-go token：关掉 LoginStatus 解析（默认 guard 在无登录态时返回 401，不 panic）。
		WithoutMiddleware(LoginStatus),
		WithAuthPolicy(NewAuthPolicy(loginGate, employeeGate)),
	)
	scope := svr.Scope("/api/v1")
	scope.PRequireLogin("/file.upload", handler)
	scope.PRequireEmployee("/admin.export", handler)
	scope.GRequireLogin("/user.session", handler)

	// 匿名：三个糖分别命中各自的 guard，响应形态由策略自己决定。
	anonymous := []struct {
		method string
		path   string
		want   string
	}{
		{http.MethodPost, "/api/v1/file.upload", "login-gate-denied"},
		{http.MethodPost, "/api/v1/admin.export", "employee-gate-denied"},
		{http.MethodGet, "/api/v1/user.session", "login-gate-denied"},
	}
	for _, testCase := range anonymous {
		rec := doRequest(svr, testCase.method, testCase.path, nil, nil)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), testCase.want) {
			t.Fatalf("%s %s: status=%d body=%q, want custom gate body %q",
				testCase.method, testCase.path, rec.Code, rec.Body.String(), testCase.want)
		}
	}

	// 通过登录闸：PRequireLogin / GRequireLogin 放行；员工闸仍然拦。
	login := map[string]string{"X-Login-Gate": "1"}
	rec := doRequest(svr, http.MethodPost, "/api/v1/file.upload", login, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "policy-ok" {
		t.Fatalf("PRequireLogin with custom gate: status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = doRequest(svr, http.MethodGet, "/api/v1/user.session", login, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "policy-ok" {
		t.Fatalf("GRequireLogin with custom gate: status=%d body=%q", rec.Code, rec.Body.String())
	}
	rec = doRequest(svr, http.MethodPost, "/api/v1/admin.export", login, nil)
	if !strings.Contains(rec.Body.String(), "employee-gate-denied") {
		t.Fatalf("PRequireEmployee must use policy.Employee(), got status=%d body=%q", rec.Code, rec.Body.String())
	}

	// 通过员工闸：PRequireEmployee 放行。
	rec = doRequest(svr, http.MethodPost, "/api/v1/admin.export", map[string]string{"X-Employee-Gate": "1"}, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "policy-ok" {
		t.Fatalf("PRequireEmployee with custom gate: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

// TestAuthPolicyPerPrefix 覆盖：同一 Router 内不同前缀用不同账号体系（多版本并存场景）。
func TestAuthPolicyPerPrefix(t *testing.T) {
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "prefix-ok") }

	svr := NewRouter(
		WithPrefixes("/v3", "/v4"),
		WithAuthPolicy(NewAuthPolicy(
			requireHeaderGate("X-Legacy-Token", "legacy-denied", "legacy login required"),
			requireHeaderGate("X-Legacy-Token", "legacy-denied", "legacy login required"),
		)),
		WithPrefixAuthPolicy("/v4", NewAuthPolicy(
			requireHeaderGate("X-Session-Cookie", "session-denied", "session required"),
			requireHeaderGate("X-Session-Cookie", "session-denied", "session required"),
		)),
	)
	// 同一 path 在 v3/v4 各自用各自的账号体系与实现。
	svr.Scope("/v3").PRequireLogin("/user.profile", handler)
	svr.Scope("/v4").PRequireLogin("/user.profile", handler)

	cases := []struct {
		name   string
		path   string
		header string
		want   string
	}{
		{"v3 without legacy token", "/v3/user.profile", "", "legacy-denied"},
		{"v3 with legacy token", "/v3/user.profile", "X-Legacy-Token", "prefix-ok"},
		{"v3 rejects v4 session", "/v3/user.profile", "X-Session-Cookie", "legacy-denied"},
		{"v4 without session", "/v4/user.profile", "", "session-denied"},
		{"v4 with session", "/v4/user.profile", "X-Session-Cookie", "prefix-ok"},
		{"v4 rejects legacy token", "/v4/user.profile", "X-Legacy-Token", "session-denied"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			var hdrs map[string]string
			if testCase.header != "" {
				hdrs = map[string]string{testCase.header: "1"}
			}
			rec := doRequest(svr, http.MethodPost, testCase.path, hdrs, nil)
			if !strings.Contains(rec.Body.String(), testCase.want) {
				t.Fatalf("%s: status=%d body=%q, want body containing %q", testCase.path, rec.Code, rec.Body.String(), testCase.want)
			}
		})
	}
}

// TestAuthPolicyDefaults 覆盖：NewAuthPolicy(nil, nil) 与默认策略等价（回退到 yg-go 内置鉴权）。
func TestAuthPolicyDefaults(t *testing.T) {
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "default-ok") }

	svr := NewRouter(WithPrefixes("/v3"), WithAuthPolicy(NewAuthPolicy(nil, nil)))
	svr.Scope("/v3").PRequireLogin("/user.profile", handler)
	svr.Scope("/v3").PRequireEmployee("/admin.profile", handler)

	for _, path := range []string{"/v3/user.profile", "/v3/admin.profile"} {
		rec := doRequest(svr, http.MethodPost, path, nil, nil)
		if !strings.Contains(rec.Body.String(), `"code":401`) || !strings.Contains(rec.Body.String(), "unauthorized") {
			t.Fatalf("%s: nil fallback should keep builtin 401 envelope, got status=%d body=%q", path, rec.Code, rec.Body.String())
		}
	}
}

// TestAuthPolicyOptionPanics 覆盖构造期失败语义：nil 策略、未声明前缀、sealed 后应用。
func TestAuthPolicyOptionPanics(t *testing.T) {
	msg := recoverPanic(t, func() { NewRouter(WithPrefixes("/v3"), WithAuthPolicy(nil)) })
	requireContains(t, msg, "WithAuthPolicy", "must not be nil")

	msg = recoverPanic(t, func() {
		NewRouter(WithPrefixes("/v3"), WithPrefixAuthPolicy("/v9", NewAuthPolicy(nil, nil)))
	})
	requireContains(t, msg, "WithPrefixAuthPolicy", "undeclared prefix", "/v9")

	msg = recoverPanic(t, func() {
		NewRouter(WithPrefixes("/v3"), WithPrefixAuthPolicy("  ", NewAuthPolicy(nil, nil)))
	})
	requireContains(t, msg, "prefix must not be blank")

	svr := NewRouter(WithPrefixes("/v3"))
	sealed := WithAuthPolicy(DefaultAuthPolicy())
	requireContains(t, recoverPanic(t, func() { sealed(svr) }), "sealed")
}

// TestAuthPolicyDefaultWithoutLoginStatus 钉住「关掉 LoginStatus + 默认策略」的真实语义：
// 默认 guard 应 fail-closed 返回 401，而不是 MustGet panic 被 Recovery 兜成 500。
func TestAuthPolicyDefaultWithoutLoginStatus(t *testing.T) {
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "should-not-reach") }
	svr := NewRouter(WithPrefixes("/v3"), WithoutMiddleware(LoginStatus))
	scope := svr.Scope("/v3")
	scope.PRequireLogin("/login.probe", handler)
	scope.PRequireEmployee("/employee.probe", handler)

	for _, path := range []string{"/v3/login.probe", "/v3/employee.probe"} {
		rec := doRequest(svr, http.MethodPost, path, nil, nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status=%d, want 200 (401 envelope)", path, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, `"code":401`) || strings.Contains(body, `"code":500`) {
			t.Fatalf("%s: body=%q, want code 401 and not 500", path, body)
		}
	}
}

// nilLoginPolicy 只实现 Employee，Login 返回 nil，用来验证注册期就能发现策略实现不完整。
type nilLoginPolicy struct{}

func (nilLoginPolicy) Login() gin.HandlerFunc { return nil }
func (nilLoginPolicy) Employee() gin.HandlerFunc {
	return func(ctx *gin.Context) { ctx.Next() }
}

// nilEmployeePolicy 只实现 Login，Employee 返回 nil。
type nilEmployeePolicy struct{}

func (nilEmployeePolicy) Login() gin.HandlerFunc {
	return func(ctx *gin.Context) { ctx.Next() }
}
func (nilEmployeePolicy) Employee() gin.HandlerFunc { return nil }

// TestAuthPolicyNilGuardPanics 覆盖策略返回 nil guard：注册期 panic，而不是把 nil 带进
// handler 链让每个请求 500。
func TestAuthPolicyNilGuardPanics(t *testing.T) {
	cases := []struct {
		name   string
		policy AuthPolicy
		action string
	}{
		{name: "nil login", policy: nilLoginPolicy{}, action: "login"},
		{name: "nil employee", policy: nilEmployeePolicy{}, action: "employee"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			svr := NewRouter(WithPrefixes("/v3"), WithAuthPolicy(testCase.policy))
			scope := svr.Scope("/v3")
			msg := recoverPanic(t, func() {
				if testCase.action == "login" {
					scope.PRequireLogin("/x", func(ctx *gin.Context) {})
					return
				}
				scope.PRequireEmployee("/x", func(ctx *gin.Context) {})
			})
			requireContains(t, msg, "returned nil", "AuthPolicy")
		})
	}
}
