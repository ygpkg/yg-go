package server

import (
	"bytes"
	"fmt"
	"net/http"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime/middleware"
	"github.com/ygpkg/yg-go/logs"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 本文件固化 v1.27 中间件链契约：8 个内置条目的固定顺序（Recovery 链首、
// IdentityFields 末位）、WithMiddleware 尾插晚于 IdentityFields、内置项可关/可替换/
// 可按锚点插入、Logger 白名单可覆盖。
// 顺序断言同时看「真实进链的 handler 函数名」与 mw.builtNames()，两者必须一致。

// chainFuncName 返回 handler 的函数全名，用于断言链中真实条目。
func chainFuncName(handler gin.HandlerFunc) string {
	if handler == nil {
		return ""
	}
	return runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name()
}

// chainIndexByName 从 from 开始查找函数名包含 token 的链下标，未命中返回 -1。
func chainIndexByName(handlers gin.HandlersChain, token string, from int) int {
	for i := from; i < len(handlers); i++ {
		if strings.Contains(chainFuncName(handlers[i]), token) {
			return i
		}
	}
	return -1
}

// bufferLoggerMiddleware 在链上装一个写进 buf 的上下文 logger，
// 供「访问日志是否被白名单跳过」「尾插时身份字段是否已写入」这类行为断言使用。
func bufferLoggerMiddleware(buf *bytes.Buffer) gin.HandlerFunc {
	encoderConfig := zap.NewProductionEncoderConfig()
	core := zapcore.NewCore(zapcore.NewJSONEncoder(encoderConfig), zapcore.AddSync(buf), zap.InfoLevel)
	sugar := zap.New(core).Sugar()
	return func(ctx *gin.Context) {
		logs.SetContextLogger(ctx, sugar)
		ctx.Next()
	}
}

// bufferLines 把日志缓冲切成非空行。
func bufferLines(buf *bytes.Buffer) []string {
	var lines []string
	for _, line := range strings.Split(buf.String(), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
		}
	}
	return lines
}

// bufferLineWith 返回第一条包含 substr 的日志行，找不到即失败。
func bufferLineWith(t *testing.T, buf *bytes.Buffer, substr string) string {
	t.Helper()
	for _, line := range bufferLines(buf) {
		if strings.Contains(line, substr) {
			return line
		}
	}
	t.Fatalf("no log line contains %q, got:\n%s", substr, buf.String())
	return ""
}

// TestChainOrder 覆盖验收「默认链顺序」：8 项固定顺序，Recovery 链首，IdentityFields 末位。
func TestChainOrder(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3"))

	wantNames := []string{
		"recovery", "cors", "customer_header", "logger",
		"login_status", "language", "inject", "identity_fields",
	}
	if got := svr.mw.builtNames(); !reflect.DeepEqual(got, wantNames) {
		t.Fatalf("builtNames() = %v, want %v", got, wantNames)
	}

	handlers := svr.GinEngine().Handlers
	if len(handlers) != len(wantNames) {
		t.Fatalf("engine chain has %d handlers, want %d builtin entries", len(handlers), len(wantNames))
	}

	// D2 不变量：Recovery 必须在链首，否则 CORS/CustomerHeader/Logger 的 panic 兜不住。
	if name := chainFuncName(handlers[0]); !strings.Contains(name, ".Recovery") {
		t.Fatalf("chain head = %q, want function containing .Recovery", name)
	}
	// IdentityFields 必须是框架中间件的最后一环。
	if name := chainFuncName(handlers[len(handlers)-1]); !strings.Contains(name, ".IdentityFields") {
		t.Fatalf("chain tail = %q, want function containing .IdentityFields", name)
	}

	// 其余位置用真实进链函数名逐项校验，防止 builtNames() 与 build() 脱节。
	tokens := []string{
		"middleware.Recovery",
		"cors.New",
		"middleware.CustomerHeader",
		"middleware.Logger",
		"middleware.LoginStatus",
		"middleware.AcceptLanguage",
		".Inject",
		"middleware.IdentityFields",
	}
	for i, token := range tokens {
		if name := chainFuncName(handlers[i]); !strings.Contains(name, token) {
			t.Fatalf("engine chain[%d] = %q, want function containing %q", i, name, token)
		}
	}
}

// TestTailInsertAfterIdentityFields 覆盖验收「尾插位置契约」：
// WithMiddleware 挂的中间件必须晚于 IdentityFields，且执行时身份字段已写入请求日志上下文。
func TestTailInsertAfterIdentityFields(t *testing.T) {
	buf := &bytes.Buffer{}
	tailRan := false
	tail := func(ctx *gin.Context) {
		tailRan = true
		logs.InfoContextf(ctx, "tail-after-identity-fields")
		ctx.Next()
	}

	svr := NewRouter(WithPrefixes("/v3"),
		// 捕获点必须在 Logger 之前：IdentityFields 之后的业务日志都用这个 context logger。
		WithMiddlewareBefore(Logger, bufferLoggerMiddleware(buf)),
		WithMiddleware(tail),
	)
	svr.RouterGroup("/v3").G("/tail.probe", func(ctx *gin.Context) { ctx.String(http.StatusOK, "tail-ok") })

	handlers := svr.GinEngine().Handlers
	// 链 = 8 个内置项 + 1 个 WithMiddlewareBefore 插入项（日志捕获）+ 1 个 WithMiddleware 尾插项。
	builtinCount := len(svr.mw.builtNames())
	if len(handlers) != builtinCount+2 {
		t.Fatalf("chain has %d handlers, want %d builtins + 1 before-insert + 1 tail", len(handlers), builtinCount)
	}
	tailIdx := len(handlers) - 1
	if name := chainFuncName(handlers[tailIdx]); !strings.Contains(name, "TestTailInsertAfterIdentityFields") {
		t.Fatalf("chain tail = %q, want the WithMiddleware tail middleware", name)
	}
	identityIdx := chainIndexByName(handlers, "middleware.IdentityFields", 0)
	if identityIdx < 0 || identityIdx >= tailIdx {
		t.Fatalf("tail middleware index = %d, must be after IdentityFields index = %d", tailIdx, identityIdx)
	}

	rec := doRequest(svr, http.MethodGet, "/v3/tail.probe", nil, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "tail-ok" {
		t.Fatalf("/v3/tail.probe: status=%d body=%q", rec.Code, rec.Body.String())
	}
	if !tailRan {
		t.Fatal("tail middleware did not run")
	}
	line := bufferLineWith(t, buf, "tail-after-identity-fields")
	// companyid/uin 由 IdentityFields 写进 context logger；尾插读到说明它确实排在 IdentityFields 之后。
	if !strings.Contains(line, `"companyid":`) || !strings.Contains(line, `"uin":`) {
		t.Fatalf("tail middleware logged before IdentityFields wrote identity fields: %s", line)
	}
}

// TestDisableBuiltin 覆盖验收「内置中间件可关」：
// WithoutMiddleware(CORS) 后带 Origin 的响应不再有 CORS 头，启动日志侧由 disabledNames() 体现。
func TestDisableBuiltin(t *testing.T) {
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "cors-ok") }
	origin := map[string]string{"Origin": "https://frontend.example"}

	enabled := NewRouter(WithPrefixes("/v3"))
	enabled.RouterGroup("/v3").G("/cors.probe", handler)
	if got := doRequest(enabled, http.MethodGet, "/v3/cors.probe", origin, nil).Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Fatalf("default chain should emit CORS header, got %q", got)
	}

	disabled := NewRouter(WithPrefixes("/v3"), WithoutMiddleware(CORS))
	disabled.RouterGroup("/v3").G("/cors.probe", handler)
	if got := doRequest(disabled, http.MethodGet, "/v3/cors.probe", origin, nil).Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Fatalf("WithoutMiddleware(CORS) should drop CORS header, got %q", got)
	}

	if got := disabled.mw.disabledNames(); !reflect.DeepEqual(got, []string{"cors"}) {
		t.Fatalf("disabledNames() = %v, want [cors]", got)
	}
	// 其余内置项相对顺序不变，被禁用项不再出现在 built 里。
	wantRemaining := []string{
		"recovery", "customer_header", "logger",
		"login_status", "language", "inject", "identity_fields",
	}
	if got := disabled.mw.builtNames(); !reflect.DeepEqual(got, wantRemaining) {
		t.Fatalf("builtNames() after disabling CORS = %v, want %v", got, wantRemaining)
	}
	for _, handler := range disabled.GinEngine().Handlers {
		if strings.Contains(chainFuncName(handler), "cors.New") {
			t.Fatalf("CORS handler still in chain: %q", chainFuncName(handler))
		}
	}
}

// TestMiddlewareImplReplaces 覆盖验收「内置项可替换」：WithMiddlewareImpl 用自定义实现替换，链里只剩一份。
func TestMiddlewareImplReplaces(t *testing.T) {
	custom := func(ctx *gin.Context) {
		ctx.Header("X-Custom-Recovery", "1")
		ctx.Next()
	}
	svr := NewRouter(WithPrefixes("/v3"), WithMiddlewareImpl(Recovery, custom))
	svr.RouterGroup("/v3").G("/impl.probe", func(ctx *gin.Context) { ctx.String(http.StatusOK, "impl-ok") })

	handlers := svr.GinEngine().Handlers
	if len(handlers) != len(svr.mw.builtNames()) {
		t.Fatalf("chain length %d != builtNames length %d", len(handlers), len(svr.mw.builtNames()))
	}
	if name := chainFuncName(handlers[0]); !strings.Contains(name, "TestMiddlewareImplReplaces") {
		t.Fatalf("chain head = %q, want the custom impl", name)
	}
	for i, handler := range handlers {
		if strings.Contains(chainFuncName(handler), "middleware.Recovery") {
			t.Fatalf("default Recovery implementation still in chain at index %d", i)
		}
	}
	// 只替换实现，条目本身仍在 built 队列里且只出现一次。
	names := svr.mw.builtNames()
	recoveryCount := 0
	for _, name := range names {
		if name == "recovery" {
			recoveryCount++
		}
	}
	if len(names) != 8 || recoveryCount != 1 || names[0] != "recovery" {
		t.Fatalf("builtNames() = %v, want 8 entries with recovery exactly once at head", names)
	}

	rec := doRequest(svr, http.MethodGet, "/v3/impl.probe", nil, nil)
	if rec.Code != http.StatusOK || rec.Body.String() != "impl-ok" {
		t.Fatalf("/v3/impl.probe: status=%d body=%q", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("X-Custom-Recovery"); got != "1" {
		t.Fatalf("custom impl did not run, header = %q", got)
	}
}

// TestMiddlewareBeforeAnchorPanics 覆盖失败语义：锚点不存在、锚点 == Recovery、
// 锚点已被 WithoutMiddleware 关掉，三种都在构造期 panic。
func TestMiddlewareBeforeAnchorPanics(t *testing.T) {
	next := func(ctx *gin.Context) { ctx.Next() }

	cases := []struct {
		name         string
		opts         []RouterOption
		wantContains string
	}{
		{
			name:         "unknown anchor",
			opts:         []RouterOption{WithPrefixes("/v3"), WithMiddlewareBefore(MwEntry{}, next)},
			wantContains: "unknown middleware",
		},
		{
			name:         "recovery anchor",
			opts:         []RouterOption{WithPrefixes("/v3"), WithMiddlewareBefore(Recovery, next)},
			wantContains: "must stay first",
		},
		{
			name: "disabled anchor",
			opts: []RouterOption{
				WithPrefixes("/v3"),
				WithoutMiddleware(LoginStatus),
				WithMiddlewareBefore(LoginStatus, next),
			},
			wantContains: "disabled by WithoutMiddleware",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			msg := recoverPanic(t, func() { _ = NewRouter(testCase.opts...) })
			requireContains(t, msg, testCase.wantContains)
		})
	}

	// 对照：合法锚点把中间件插在目标项之前，且不改变其余顺序。
	t.Run("valid anchor inserts before target", func(t *testing.T) {
		svr := NewRouter(WithPrefixes("/v3"), WithMiddlewareBefore(Language, next))
		handlers := svr.GinEngine().Handlers
		insertedIdx := chainIndexByName(handlers, "TestMiddlewareBeforeAnchorPanics", 0)
		languageIdx := chainIndexByName(handlers, "middleware.AcceptLanguage", 0)
		if insertedIdx < 0 {
			t.Fatalf("inserted middleware not found in chain")
		}
		if languageIdx != insertedIdx+1 {
			t.Fatalf("inserted middleware index = %d, AcceptLanguage index = %d, want adjacent", insertedIdx, languageIdx)
		}
		if len(handlers) != 9 {
			t.Fatalf("chain has %d handlers, want 8 builtins + 1 inserted", len(handlers))
		}
	})
}

// TestLoggerWhitelistOption 覆盖验收「Logger 白名单可配置」：
// 白名单通过 middleware.Logger 的选项表达（WithLoggerWhitelist），
// 用 WithMiddlewareImpl 替换内置 Logger 条目，命中白名单的路由跳过访问日志。
func TestLoggerWhitelistOption(t *testing.T) {
	handler := func(ctx *gin.Context) { ctx.String(http.StatusOK, "ok") }

	newProbe := func(opts ...middleware.LoggerOption) (*Router, *bytes.Buffer) {
		buf := &bytes.Buffer{}
		svr := NewRouter(WithPrefixes("/v3"),
			// 捕获点必须在内置 Logger 之前，才能拿到访问日志
			WithMiddlewareBefore(Logger, bufferLoggerMiddleware(buf)),
			WithMiddlewareImpl(Logger, middleware.Logger(opts...)),
		)
		v3 := svr.RouterGroup("/v3")
		v3.G("/healthz", handler)
		v3.G("/user/detail", handler)
		v3.G("/canvas.Ping", handler) // 默认白名单的后缀命中路径
		return svr, buf
	}

	hit := func(t *testing.T, svr *Router, buf *bytes.Buffer, path string) bool {
		t.Helper()
		if rec := doRequest(svr, http.MethodGet, path, nil, nil); rec.Code != http.StatusOK {
			t.Fatalf("%s: status=%d", path, rec.Code)
		}
		return strings.Contains(buf.String(), path)
	}

	t.Run("override", func(t *testing.T) {
		svr, buf := newProbe(middleware.WithLoggerWhitelist("/healthz"))
		if hit(t, svr, buf, "/v3/healthz") {
			t.Fatalf("whitelisted route should skip access log, got:\n%s", buf.String())
		}
		if !hit(t, svr, buf, "/v3/user/detail") {
			t.Fatalf("non-whitelisted route should keep access log, got:\n%s", buf.String())
		}
		// 覆盖语义：默认白名单里的 ".Ping" 被替换掉，不再跳过。
		if !hit(t, svr, buf, "/v3/canvas.Ping") {
			t.Fatalf("WithLoggerWhitelist should replace default whitelist, .Ping still skipped, got:\n%s", buf.String())
		}
	})

	t.Run("default", func(t *testing.T) {
		buf := &bytes.Buffer{}
		svr := NewRouter(WithPrefixes("/v3"),
			WithMiddlewareBefore(Logger, bufferLoggerMiddleware(buf)),
		)
		v3 := svr.RouterGroup("/v3")
		v3.G("/canvas.Ping", handler)
		v3.G("/user/detail", handler)
		if hit(t, svr, buf, "/v3/canvas.Ping") {
			t.Fatalf("default whitelist should skip .Ping suffix, got:\n%s", buf.String())
		}
		if !hit(t, svr, buf, "/v3/user/detail") {
			t.Fatalf("default whitelist should keep access log, got:\n%s", buf.String())
		}
	})
}

// TestMiddlewareDisabledImplConflictPanics 覆盖「同一条目既禁用又替换」：装配期 panic，
// 而不是静默保留禁用、丢掉替换（与 D11「配置错误启动期失败」一致）。
func TestMiddlewareDisabledImplConflictPanics(t *testing.T) {
	custom := func(ctx *gin.Context) { ctx.Next() }
	optionSets := map[string][]RouterOption{
		"disable then impl": {
			WithPrefixes("/v3"),
			WithoutMiddleware(CORS),
			WithMiddlewareImpl(CORS, custom),
		},
		"impl then disable": {
			WithPrefixes("/v3"),
			WithMiddlewareImpl(CORS, custom),
			WithoutMiddleware(CORS),
		},
	}
	for name, opts := range optionSets {
		t.Run(name, func(t *testing.T) {
			msg := recoverPanic(t, func() { _ = NewRouter(opts...) })
			requireContains(t, msg, "disabled by WithoutMiddleware", "WithMiddlewareImpl")
		})
	}
}

// TestAssemblySummaryAndDisabledConsequences 断言启动日志内容（摘要 + 禁用项后果），
// 不依赖抓 stdout：logAssembly 打印的摘要由纯函数 assemblySummary 产生。
func TestAssemblySummaryAndDisabledConsequences(t *testing.T) {
	svr := NewRouter(WithPrefixes("/v3", "/v4"), WithoutMiddleware(CORS))
	// 用真实链长度断言，避免「常量对常量」的自证。
	chainLen := len(svr.GinEngine().Handlers)
	summary := svr.assemblySummary(chainLen)
	requireContains(t, summary, "prefixes=[/v3 /v4]", fmt.Sprintf("chain_len=%d", chainLen), "builtin=")
	if strings.Contains(summary, "cors") {
		t.Fatalf("summary should not list disabled cors, got %q", summary)
	}
	if got := svr.mw.disabledNames(); !reflect.DeepEqual(got, []string{"cors"}) {
		t.Fatalf("disabledNames() = %v, want [cors]", got)
	}
	for _, entry := range builtinMwOrder {
		if got := mwDisabledConsequence(entry.name); got == "" {
			t.Fatalf("mwDisabledConsequence(%q) is empty", entry.name)
		}
	}
}

// TestDefaultLoggerWhitelistCopy 覆盖 DefaultLoggerWhitelist 返回副本，调用方 append/修改
// 不会污染内置条目默认值。
func TestDefaultLoggerWhitelistCopy(t *testing.T) {
	whitelist := DefaultLoggerWhitelist()
	if want := []string{".Ping", "metrics"}; !reflect.DeepEqual(whitelist, want) {
		t.Fatalf("DefaultLoggerWhitelist() = %v, want %v", whitelist, want)
	}
	whitelist[0] = "mutated"
	if got := DefaultLoggerWhitelist()[0]; got != ".Ping" {
		t.Fatalf("DefaultLoggerWhitelist() leaked internal slice, got %q", got)
	}
}

// TestDisableEachBuiltinKeepsOrder 覆盖验收「任意关掉一项，其余项相对顺序不变」。
func TestDisableEachBuiltinKeepsOrder(t *testing.T) {
	defaults := builtinNames()
	for _, entry := range builtinMwOrder {
		svr := NewRouter(WithPrefixes("/v3"), WithoutMiddleware(entry))
		want := make([]string, 0, len(defaults))
		for _, name := range defaults {
			if name != entry.name {
				want = append(want, name)
			}
		}
		if got := svr.mw.builtNames(); !reflect.DeepEqual(got, want) {
			t.Fatalf("disable %s: builtNames = %v, want %v", entry.name, got, want)
		}
	}
}
