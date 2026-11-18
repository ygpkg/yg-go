package server

import (
	"fmt"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime/middleware"
)

// MwEntry 是一个内置中间件的「条目定义」。
//
// 调用方直接引用条目本身（server.CORS / server.Logger / ...），而不是名字字符串。
// 导出的条目变量是只读的：不要给 server.Logger 等变量重新赋值（会静默改变所有 Router 的装配结果）。
//
//	server.WithoutMiddleware(server.CORS)                              // 关闭 CORS
//	server.WithMiddlewareBefore(server.Logger, traceMW)                // 在访问日志之前插入
//	server.WithMiddlewareImpl(server.Logger, middleware.Logger(        // 替换实现并带选项
//	    middleware.WithLoggerWhitelist(".Ping", "metrics", "/healthz"),
//	))
//
// 条目字段不可构造，包外只能使用下面导出的条目（或其副本）：拼写错误由编译器拦住，也不会出现
// 「字符串对不上 → 配置静默不生效」。中间件自身的可配置项（Logger 白名单、CORS 配置等）
// 由 middleware 包以 functional options 表达，而不是在 Router 上另开一批开关。
type MwEntry struct {
	name  string
	build func(*Router) gin.HandlerFunc
}

// 内置条目名（内部使用：装配、日志、配置索引）。
const (
	mwNameRecovery       = "recovery"
	mwNameCORS           = "cors"
	mwNameCustomerHeader = "customer_header"
	mwNameLogger         = "logger"
	mwNameLoginStatus    = "login_status"
	mwNameLanguage       = "language"
	mwNameInject         = "inject"
	mwNameIdentityFields = "identity_fields"
)

// 内置中间件条目。默认装配顺序见 builtinMwOrder。
var (
	// Recovery panic 兜底并返回统一错误；固定链首（D2）。
	Recovery = MwEntry{mwNameRecovery, func(*Router) gin.HandlerFunc { return middleware.Recovery() }}
	// CORS 跨域响应头；默认配置与 1.23 线一致，用 middleware.CORS(middleware.WithCORS...(...)) 做定向扩展。
	CORS = MwEntry{mwNameCORS, func(*Router) gin.HandlerFunc { return middleware.CORS() }}
	// CustomerHeader 注入 reqid、回写 Env 响应头。
	CustomerHeader = MwEntry{mwNameCustomerHeader, func(*Router) gin.HandlerFunc { return middleware.CustomerHeader() }}
	// Logger 访问日志与请求直方图；默认白名单 {".Ping", "metrics"}（1.23 线行为），
	// 需要覆盖时用 WithMiddlewareImpl(server.Logger, middleware.Logger(...)) 传选项；
	// 想保留默认项再加自己的路径，用 DefaultLoggerWhitelist() + append。
	Logger = MwEntry{mwNameLogger, func(*Router) gin.HandlerFunc {
		return middleware.Logger(middleware.WithLoggerWhitelist(defaultLoggerWhitelist...))
	}}
	// LoginStatus 解析 token 写入登录态。
	LoginStatus = MwEntry{mwNameLoginStatus, func(*Router) gin.HandlerFunc { return middleware.LoginStatus() }}
	// Language 写入 Accept-Language。
	Language = MwEntry{mwNameLanguage, func(*Router) gin.HandlerFunc { return middleware.AcceptLanguage() }}
	// Inject 按 issuer 调用鉴权注入器（引擎级注入器的注册入口是 Router.AuthInject / Default）。
	Inject = MwEntry{mwNameInject, func(svr *Router) gin.HandlerFunc { return svr.Inject }}
	// IdentityFields 把 companyid/uin 写进请求日志上下文；固定末位且晚于 Inject。
	IdentityFields = MwEntry{mwNameIdentityFields, func(*Router) gin.HandlerFunc { return middleware.IdentityFields() }}
)

// defaultLoggerWhitelist 是内置 Logger 条目的默认白名单（与 1.23 线写死在 server 里的一致）。
var defaultLoggerWhitelist = []string{".Ping", "metrics"}

// DefaultLoggerWhitelist 返回内置 Logger 条目的默认白名单副本。
//
// 用 WithMiddlewareImpl(server.Logger, middleware.Logger(...)) 替换实现时，用它保留默认项：
//
//	whitelist := append(server.DefaultLoggerWhitelist(), "/healthz")
//	server.WithMiddlewareImpl(server.Logger,
//	    middleware.Logger(middleware.WithLoggerWhitelist(whitelist...)))
func DefaultLoggerWhitelist() []string {
	whitelist := make([]string, len(defaultLoggerWhitelist))
	copy(whitelist, defaultLoggerWhitelist)
	return whitelist
}

// builtinMwOrder 是内置条目的默认装配顺序。
//
// 两条不变量由代码（而不是注释）保证：
//   - Recovery 在链首：CORS、CustomerHeader、Logger 三者内部 panic 也能被兜住
//     （1.23 线是 CORS → CustomerHeader → Logger → Recovery，前三者的 panic 直冲 net/http）。
//   - 其余项保持 1.23 线相对顺序，IdentityFields 在末位、且晚于 Inject：
//     companyid/uin 由各 app 的 auth injector 在 Inject 里写进 idmap，读早了恒为 0。
var builtinMwOrder = []MwEntry{
	Recovery,
	CORS,
	CustomerHeader,
	Logger,
	LoginStatus,
	Language,
	Inject,
	IdentityFields,
}

// middlewareConfig 是中间件链的构造期配置。NewRouter 返回之后不可变。
type middlewareConfig struct {
	// disabled 通过 WithoutMiddleware 关闭的内置条目名。
	disabled map[string]bool
	// impls 通过 WithMiddlewareImpl 替换实现的条目名 → 中间件。
	impls map[string]gin.HandlerFunc
	// before 在某个条目之前插入的中间件（条目名 → 按声明顺序的链）。
	before map[string][]gin.HandlerFunc
	// tail 通过 WithMiddleware 追加到整条链之后的中间件（晚于 IdentityFields）。
	tail []gin.HandlerFunc
	// built 实际进链的内置条目名，按装配顺序排列，由 build 回填。
	built []string
}

// newMiddlewareConfig 返回默认配置：8 个条目全开，Logger/CORS 用各自默认选项。
func newMiddlewareConfig() *middlewareConfig {
	return &middlewareConfig{
		disabled: make(map[string]bool),
		impls:    make(map[string]gin.HandlerFunc),
		before:   make(map[string][]gin.HandlerFunc),
	}
}

// build 按 builtinMwOrder 装配引擎级中间件链。
//
// 装配规则：按固定顺序遍历，禁用的跳过；锚点插入物排在该条目之前；有 impl 用 impl、
// 否则用条目定义的默认构造；最后拼接 tail（WithMiddleware，永远晚于 IdentityFields）。
func (cfg *middlewareConfig) build(svr *Router) []gin.HandlerFunc {
	cfg.validate()

	chain := make([]gin.HandlerFunc, 0, len(builtinMwOrder)+len(cfg.tail))
	built := make([]string, 0, len(builtinMwOrder))
	for _, entry := range builtinMwOrder {
		if cfg.disabled[entry.name] {
			continue
		}
		if inserts := cfg.before[entry.name]; len(inserts) > 0 {
			chain = append(chain, inserts...)
		}
		chain = append(chain, cfg.resolve(svr, entry))
		built = append(built, entry.name)
	}
	chain = append(chain, cfg.tail...)
	cfg.built = built
	return chain
}

// resolve 返回某个条目的实现：优先用 WithMiddlewareImpl 的替换项，否则用条目定义。
func (cfg *middlewareConfig) resolve(svr *Router, entry MwEntry) gin.HandlerFunc {
	if impl, ok := cfg.impls[entry.name]; ok {
		return impl
	}
	return entry.build(svr)
}

// builtNames 返回实际进链的内置条目名，按 builtinMwOrder 的稳定顺序输出。
func (cfg *middlewareConfig) builtNames() []string {
	names := make([]string, len(cfg.built))
	copy(names, cfg.built)
	return names
}

// disabledNames 返回被关闭的内置条目名，按 builtinMwOrder 的稳定顺序输出。
func (cfg *middlewareConfig) disabledNames() []string {
	names := make([]string, 0, len(cfg.disabled))
	for _, entry := range builtinMwOrder {
		if cfg.disabled[entry.name] {
			names = append(names, entry.name)
		}
	}
	return names
}

// validate 校验条目配置，任何非法组合都在装配期 panic（早于监听端口）。
//
// 三类错误：条目不属于内置 8 项（传了零值或拼错）；before 锚点等于 Recovery
// （会破坏「Recovery 链首」的 D2 不变量）；before 锚点已被 WithoutMiddleware 关掉
// （插在一个不进链的位置上，语义无从谈起）。
func (cfg *middlewareConfig) validate() {
	for _, name := range sortedNames(cfg.disabled) {
		if !isBuiltinName(name) {
			panicUnknownMiddleware("WithoutMiddleware", MwEntry{name: name})
		}
	}
	for _, name := range sortedNames(cfg.impls) {
		if !isBuiltinName(name) {
			panicUnknownMiddleware("WithMiddlewareImpl", MwEntry{name: name})
		}
	}
	for _, name := range sortedNames(cfg.before) {
		if !isBuiltinName(name) {
			panicUnknownMiddleware("WithMiddlewareBefore", MwEntry{name: name})
		}
	}
	for _, entry := range builtinMwOrder {
		if cfg.disabled[entry.name] && cfg.impls[entry.name] != nil {
			panic(fmt.Sprintf(
				"server: WithMiddlewareImpl: %s is disabled by WithoutMiddleware, remove one of them",
				mwLabel(entry)))
		}
		if len(cfg.before[entry.name]) == 0 {
			continue
		}
		validateAnchor(entry)
		if cfg.disabled[entry.name] {
			panicDisabledAnchor("WithMiddlewareBefore", entry)
		}
	}
}

// 以下 4 个链装配 option 全部改的是构造期声明，因此第一行统一调用 ensureMutable：
// NewRouter 之后再应用任一 option 会 panic，而不是静默改一个已经装配好、不再生效的配置。

// WithoutMiddleware 关闭指定的内置中间件（直接引用条目定义，如 server.CORS）。
//
// 被关闭的条目不会进链，并会在启动日志里列出「已禁用项 + 受影响 helper」，把判断责任
// 显式交给使用方（装配期无法静态判断 handler 是否依赖它写进 context 的值）。
func WithoutMiddleware(entries ...MwEntry) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		for _, entry := range entries {
			if !isBuiltinEntry(entry) {
				panicUnknownMiddleware("WithoutMiddleware", entry)
			}
			svr.mw.disabled[entry.name] = true
		}
	}
}

// WithMiddlewareImpl 替换某个内置中间件的实现（用于替换，不用于新增）。
//
// 中间件的可配置项用 middleware 包的选项表达，例如：
//
//	server.WithMiddlewareImpl(server.Logger,
//	    middleware.Logger(middleware.WithLoggerWhitelist(".Ping", "metrics", "/healthz")))
//	server.WithMiddlewareImpl(server.CORS,
//	    middleware.CORS(
//	        middleware.WithCORSAllowOrigins("https://app.example.com"),
//	        middleware.WithCORSAllowHeaders("X-Tenant-Id")))
//	// 需要完整控制（AllowWildcard / AllowPrivateNetwork 等）时直接替换成 cors.New(cfg)
//
// 同一 entry 不能同时被 WithoutMiddleware 关闭：该组合会在装配期 panic（而不是静默丢弃替换）。
// 新增中间件请用 WithMiddlewareBefore（按位置插入）或 WithMiddleware（链尾追加）。
func WithMiddlewareImpl(entry MwEntry, mw gin.HandlerFunc) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		if !isBuiltinEntry(entry) {
			panicUnknownMiddleware("WithMiddlewareImpl", entry)
		}
		if mw == nil {
			panic(fmt.Sprintf("server: WithMiddlewareImpl: middleware implementation for %s must not be nil", mwLabel(entry)))
		}
		svr.mw.impls[entry.name] = mw
	}
}

// WithMiddlewareBefore 在指定内置条目之前插入中间件（可以多次调用同一条目，按调用顺序拼接）。
//
// 锚点必须是内置条目之一、且没有被 WithoutMiddleware 关掉；锚点不能是 server.Recovery
// （Recovery 必须链首，见设计决策 D2）。
func WithMiddlewareBefore(entry MwEntry, mws ...gin.HandlerFunc) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		if !isBuiltinEntry(entry) {
			panicUnknownMiddleware("WithMiddlewareBefore", entry)
		}
		validateAnchor(entry)
		if svr.mw.disabled[entry.name] {
			panicDisabledAnchor("WithMiddlewareBefore", entry)
		}
		if inserts := nonNilHandlers(mws); len(inserts) > 0 {
			svr.mw.before[entry.name] = append(svr.mw.before[entry.name], inserts...)
		}
	}
}

// WithMiddleware 把中间件追加到链尾。
//
// 尾插永远晚于 server.IdentityFields，因此 RequestLogFields 这类「在身份字段之后追加字段」
// 的中间件能读到 companyid/uin。
func WithMiddleware(mws ...gin.HandlerFunc) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		svr.mw.tail = append(svr.mw.tail, nonNilHandlers(mws)...)
	}
}

// mwDisabledConsequence 返回禁用某个内置条目的直接后果（按设计「内置中间件与运行时依赖」表）。
//
// 装配期无法静态判断 handler 是否在用这些 helper，因此后果写进启动日志，
// 由使用方自行判断这次禁用是否安全。未知条目返回空串。
func mwDisabledConsequence(name string) string {
	switch name {
	case mwNameRecovery:
		return "未捕获的 panic 直冲 net/http，客户端表现为连接重置"
	case mwNameCORS:
		return "浏览器端跨域请求全部失败"
	case mwNameCustomerHeader:
		return "runtime.RequestID(ctx) 的 MustGet panic，BaseResponse.RequestID 为空"
	case mwNameLogger:
		return "没有访问日志，也没有 request_latency_seconds 指标"
	case mwNameLoginStatus:
		return "业务代码读 runtime.LoginStatus 会 MustGet panic（框架 Inject 已跳过）；默认 AuthPolicy 恒 401，自研账号体系请配合 WithAuthPolicy"
	case mwNameLanguage:
		return "i18n 退化为默认语言"
	case mwNameInject:
		return "鉴权注入器不执行，LoginStatus.Err 恒为空"
	case mwNameIdentityFields:
		return "访问日志退回直接从 ctx 读 uin 的旧路径，companyid 缺失"
	default:
		return ""
	}
}

// isBuiltinName 判断条目名是否属于内置 8 项之一。
func isBuiltinName(name string) bool {
	for _, entry := range builtinMwOrder {
		if entry.name == name {
			return true
		}
	}
	return false
}

// isBuiltinEntry 判断调用方传入的条目是否是内置条目（包外无法构造合法条目，这里拦住零值）。
func isBuiltinEntry(entry MwEntry) bool {
	if !isBuiltinName(entry.name) {
		return false
	}
	return entry.build != nil
}

// validateAnchor 校验锚点本身是否合法（当前只有「锚点不能是 Recovery」一条）。
func validateAnchor(entry MwEntry) {
	if entry.name == mwNameRecovery {
		panic(fmt.Sprintf(
			"server: WithMiddlewareBefore: cannot insert before anchor %s: Recovery must stay first in the middleware chain",
			mwLabel(entry)))
	}
}

// sortedNames 返回 map 的键并排序，保证多个非法项同时存在时 panic 文案稳定。
func sortedNames[T any](m map[string]T) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// nonNilHandlers 过滤 nil，避免把空 handler 带进 gin 链（运行期 nil 解引用）。
func nonNilHandlers(mws []gin.HandlerFunc) []gin.HandlerFunc {
	handlers := make([]gin.HandlerFunc, 0, len(mws))
	for _, mw := range mws {
		if mw != nil {
			handlers = append(handlers, mw)
		}
	}
	return handlers
}

// panicUnknownMiddleware 报告非法条目：启动期失败，而不是运行期静默不生效。
func panicUnknownMiddleware(op string, entry MwEntry) {
	panic(fmt.Sprintf("server: %s: unknown middleware %s, builtin middlewares are %v", op, mwLabel(entry), builtinNames()))
}

// panicDisabledAnchor 报告「锚点已被 WithoutMiddleware 关掉」。
func panicDisabledAnchor(op string, entry MwEntry) {
	panic(fmt.Sprintf(
		"server: %s: cannot insert before anchor %s: it is disabled by WithoutMiddleware, remove that call or pick another anchor",
		op, mwLabel(entry)))
}

// mwLabel 生成 panic 文案里的条目标识，给出内部名与 Go 条目名（如 `"logger" (server.Logger)`）。
func mwLabel(entry MwEntry) string {
	for _, builtin := range builtinMwOrder {
		if builtin.name == entry.name {
			return fmt.Sprintf("%q (server.%s)", entry.name, mwConstName(entry.name))
		}
	}
	return fmt.Sprintf("%q", entry.name)
}

// mwConstName 返回条目对应的 Go 变量名；未知条目返回空串。
func mwConstName(name string) string {
	switch name {
	case mwNameRecovery:
		return "Recovery"
	case mwNameCORS:
		return "CORS"
	case mwNameCustomerHeader:
		return "CustomerHeader"
	case mwNameLogger:
		return "Logger"
	case mwNameLoginStatus:
		return "LoginStatus"
	case mwNameLanguage:
		return "Language"
	case mwNameInject:
		return "Inject"
	case mwNameIdentityFields:
		return "IdentityFields"
	default:
		return ""
	}
}

// builtinNames 返回内置条目名清单（panic 文案用）。
func builtinNames() []string {
	names := make([]string, 0, len(builtinMwOrder))
	for _, entry := range builtinMwOrder {
		names = append(names, entry.name)
	}
	return names
}
