// Package server 提供 yg-go 下游服务共用的 HTTP 装配层。
//
// # 路由模型
//
// 自 v1.27 起注册入口收敛为「作用域投递」，不再有隐式广播：
//
//   - 前缀在构造期声明，精确集合：NewRouter(WithPrefixes("/v3", "/v2"))。构造参数里不再有前缀，
//     也不存在隐含的默认前缀或自动合并；需要携带前缀级中间件时用 WithPrefixGroup(s)。
//   - 注册只发生在作用域上：Router.Scope(prefix) 投递到单个前缀，Router.All() 显式广播到全部前缀。
//     Router 自身不再有任何注册方法，两者共同实现 Registrar 接口，模块路由函数只依赖该接口。
//   - 未声明前缀、重复注册、非法锚点、空前缀集合都在 Run 之前 panic，不会带着半装配的路由表启动；
//     Run 之后再注册路由也会在注册点 panic。
//
// # 中间件链
//
// 内置中间件是可直接引用的条目定义（server.Recovery / server.CORS / ...），按固定顺序装配：
// Recovery → CORS → CustomerHeader → Logger → LoginStatus → Language → Inject → IdentityFields。
// 用 WithoutMiddleware 关闭、WithMiddlewareImpl 替换、WithMiddlewareBefore 在锚点之前插入，
// WithMiddleware 追加到链尾（尾插永远晚于 IdentityFields）。链在 NewRouter 内一次成型，之后不可变。
package server

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/lifecycle"
	"github.com/ygpkg/yg-go/logs"
)

// methodAny 是 Any 注册在清单索引里的方法标识。
//
// gin 的 Any 会为同一条路径注册 9 个 HTTP 方法；Routes() 里仍以单条 "ANY" 记录表达，
// 但重复检测按 9 个方法键分别登记，因此 Any 与 P/G 混用同 path 也能在启动期给出两个注册来源。
const methodAny = "ANY"

// anyRouteMethods 是 gin Any 实际注册的 9 个方法（用于重复检测索引，顺序与 gin 一致）。
// 只读数组：与 gin 内部的 anyMethods 对齐，升级 gin 版本时需要同步核对。
var anyRouteMethods = [...]string{
	http.MethodGet,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodHead,
	http.MethodOptions,
	http.MethodDelete,
	http.MethodConnect,
	http.MethodTrace,
}

// PrefixGroup 声明一个前缀，以及只作用于该前缀的中间件。
//
// 前缀会被归一化（去空白、补前导斜杠、去尾部斜杠）后作为精确集合使用；同一前缀重复声明会去重，
// 中间件按声明顺序叠加。前缀级中间件排在引擎级中间件链之后、路由级中间件之前。
type PrefixGroup struct {
	// Prefix 前缀，如 "/v3"、"v3/"、"v4"，归一化后形如 "/v3"。
	Prefix string
	// Middlewares 该前缀下所有路由共享的中间件。
	Middlewares []gin.HandlerFunc
}

// RouteInfo 描述一条已注册的业务路由。
//
// Path 是完整注册路径（形如 "/v3/user/login"），与 gin 实际注册的 fullPath 一致；
// Source 是注册发生的位置（"file:line"），重复注册时用于定位两个来源。
// 直接挂在 GinEngine() 上的探针、NoRoute、静态文件不属于作用域模型，不会出现在 Routes() 里。
type RouteInfo struct {
	// Method HTTP 方法，如 http.MethodPost；Any 注册的条目记为 "ANY"（gin 侧对应 9 个方法）。
	Method string
	// Path 完整路径，含前缀。
	Path string
	// Prefix 该路由投递到的前缀。
	Prefix string
	// Source 注册来源，形如 "/path/to/router.go:42"。
	Source string
}

// Router 是 HTTP 装配层的对外入口。
//
// Router 只持有「声明」与「装配结果」：前缀清单、前缀规格、路由分组、清单索引与中间件链配置。
// 前缀集合与中间件链都在 NewRouter 内定死，之后不再变化，因此请求期无需加锁，也不存在
// 「后挂中间件对已有路由不生效」的时序陷阱——但注册动作仍必须发生在 Run 之前（见包文档）。
type Router struct {
	eng *gin.Engine
	// l 是 Run 传入的 listener；非 nil 表示服务已启动，之后禁止再注册路由。
	l  net.Listener
	lc *lifecycle.LifeCycle

	// prefixes 归一化、去重、按声明顺序排列的前缀清单（精确集合）。
	prefixes []string
	// prefixSpecs 归一化前缀 → 该前缀的规格（含前缀级中间件）；key 与 prefixes 一一对应。
	prefixSpecs map[string]*PrefixGroup
	// routeGroups 归一化前缀 → gin 分组；构造期一次建好，之后只读。
	routeGroups map[string]*gin.RouterGroup

	// routes method + " " + fullPath → 注册来源；只增，重复写入即 panic。
	// Any 展开成 gin 的 9 个方法键登记（RouteInfo 仍只记一条 ANY）。
	routes map[string]string
	// routeInfos 注册顺序的清单，供 Routes()/LogRoutes() 输出。
	routeInfos []RouteInfo

	// mw 具名中间件链配置；NewRouter 之后不可变。
	mw *middlewareConfig
	// authPolicy 全局鉴权策略，供 PRequireLogin/PRequireEmployee/GRequireLogin 使用。
	authPolicy AuthPolicy
	// prefixAuthPolicies 按前缀覆盖的鉴权策略（归一化前缀 → 策略）。
	prefixAuthPolicies map[string]AuthPolicy
	// sealed 标记构造期已结束，之后的声明型 option 一律 panic。
	sealed bool

	*authInjectors
}

// RouterOption 装配期选项。
//
// 所有 option 都必须在 NewRouter 的调用中应用：它们改的是构造期声明（前缀集合、中间件链），
// 在 NewRouter 返回之后再调用任一 option 会 panic。
type RouterOption func(*Router)

// WithPrefixes 声明前缀（精确集合）。
//
// 前缀按声明顺序作为注册顺序，重复声明去重；不再与构造参数或任何默认前缀合并。
// 未在声明中出现的前缀无法用于注册，Router.Scope 会 panic。
func WithPrefixes(prefixes ...string) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		for _, prefix := range prefixes {
			svr.declarePrefix(prefix, nil)
		}
	}
}

// WithPrefixGroup 声明一个前缀，并携带只作用于该前缀的中间件。
func WithPrefixGroup(prefix string, mws ...gin.HandlerFunc) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		svr.declarePrefix(prefix, mws)
	}
}

// WithPrefixGroups 批量声明前缀及其中间件。
func WithPrefixGroups(groups ...PrefixGroup) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		for _, group := range groups {
			svr.declarePrefix(group.Prefix, group.Middlewares)
		}
	}
}

// NewRouter 构造 Router：先按顺序应用 opts（前缀声明与中间件链配置），再一次性完成装配。
//
// 空前缀集合、非法中间件锚点等装配错误一律 panic：装配期错误不是运行期可恢复错误，
// 启动即失败能保证不会带着半装配的路由表上线。
func NewRouter(opts ...RouterOption) *Router {
	svr := &Router{
		eng:                gin.New(),
		lc:                 lifecycle.Std(),
		prefixSpecs:        make(map[string]*PrefixGroup),
		routeGroups:        make(map[string]*gin.RouterGroup),
		routes:             make(map[string]string),
		mw:                 newMiddlewareConfig(),
		authPolicy:         DefaultAuthPolicy(),
		prefixAuthPolicies: make(map[string]AuthPolicy),
		authInjectors: &authInjectors{
			injectors: make(map[string]auth.InjectorFunc),
			defaultInjector: func(ctx *gin.Context, ls *auth.LoginStatus) (err error) {
				return nil
			},
		},
	}
	for _, opt := range opts {
		if opt == nil {
			panic("server: RouterOption must not be nil")
		}
		opt(svr)
	}
	svr.sealed = true
	svr.assemble()
	return svr
}

// Run 在给定 listener 上启动 HTTP 服务。
//
// 路由注册必须早于 Run；Run 之后再注册会在注册点 panic。
func (svr *Router) Run(l net.Listener) error {
	if l == nil {
		panic("server: Run: listener must not be nil")
	}
	if svr.l != nil {
		panic("server: Run: already running")
	}
	svr.l = l

	go func() {
		if err := http.Serve(l, svr.eng); err != nil {
			logs.Errorf("http.Serve error: %v", err)
		}
		svr.lc.Exit()
	}()
	return nil
}

// GinEngine 返回底层 gin 引擎。
//
// 它是逃生口：探针（/healthz、/metrics）、NoRoute、静态文件这类「非业务动作」继续直接挂在 engine 上，
// 它们不进 Routes() 索引，也不参与重复检测。不要用它挂业务路由。
func (svr *Router) GinEngine() *gin.Engine {
	return svr.eng
}

// Routes 返回按注册顺序排列的业务路由清单副本。
func (svr *Router) Routes() []RouteInfo {
	routes := make([]RouteInfo, len(svr.routeInfos))
	copy(routes, svr.routeInfos)
	return routes
}

// LogRoutes 逐行打印业务路由清单（method、path、prefix、注册来源）。
func (svr *Router) LogRoutes() {
	for _, route := range svr.routeInfos {
		logs.Infof("%-7s %-48s prefix=%-8s source=%s", route.Method, route.Path, route.Prefix, route.Source)
	}
}

// ensureMutable 保证声明型 option 只作用于构造期。
func (svr *Router) ensureMutable() {
	if svr.sealed {
		panic("server: router options must be applied inside NewRouter, declaration is already sealed")
	}
}

// declarePrefix 归一化并登记前缀，同时叠加该前缀的中间件。
func (svr *Router) declarePrefix(prefix string, mws []gin.HandlerFunc) {
	normalized := normalizePrefix(prefix)
	if normalized == "" {
		panic("server: prefix must not be blank")
	}
	spec, ok := svr.prefixSpecs[normalized]
	if !ok {
		spec = &PrefixGroup{Prefix: normalized}
		svr.prefixSpecs[normalized] = spec
		svr.prefixes = append(svr.prefixes, normalized)
	}
	spec.Middlewares = append(spec.Middlewares, mws...)
}

// assemble 在 NewRouter 内一次性完成装配：中间件链、NoRoute、每个前缀的分组。
func (svr *Router) assemble() {
	if len(svr.prefixes) == 0 {
		panic("server: no prefix declared, use WithPrefixes or WithPrefixGroup")
	}
	prefixAuthKeys := make([]string, 0, len(svr.prefixAuthPolicies))
	for prefix := range svr.prefixAuthPolicies {
		prefixAuthKeys = append(prefixAuthKeys, prefix)
	}
	sort.Strings(prefixAuthKeys)
	for _, prefix := range prefixAuthKeys {
		if _, ok := svr.prefixSpecs[prefix]; !ok {
			panic(fmt.Sprintf("server: WithPrefixAuthPolicy: undeclared prefix %q, available prefixes: %v", prefix, svr.prefixes))
		}
	}
	if config.Conf().MainConf.Env != "test" {
		gin.SetMode(gin.ReleaseMode)
	}

	chain := svr.mw.build(svr)
	svr.eng.Use(chain...)
	svr.eng.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "The incorrect API route.")
	})
	for _, prefix := range svr.prefixes {
		spec := svr.prefixSpecs[prefix]
		if len(spec.Middlewares) > 0 {
			svr.routeGroups[prefix] = svr.eng.Group(prefix, spec.Middlewares...)
			continue
		}
		svr.routeGroups[prefix] = svr.eng.Group(prefix)
	}
	svr.logAssembly(len(chain))
}

// logAssembly 打印装配结果：前缀清单、链长度、实际进链的内置项、被禁内置项及其后果。
func (svr *Router) logAssembly(chainLen int) {
	logs.Infof("router assembled: %s", svr.assemblySummary(chainLen))
	for _, name := range svr.mw.disabledNames() {
		logs.Warnf("middleware %q disabled: %s", name, mwDisabledConsequence(name))
	}
}

// assemblySummary 返回装配摘要（抽成纯函数，便于单测断言启动日志内容）。
func (svr *Router) assemblySummary(chainLen int) string {
	return fmt.Sprintf("prefixes=%v chain_len=%d builtin=%v", svr.prefixes, chainLen, svr.mw.builtNames())
}

// register 是 Scope 与 AllScope 共用的注册路径：重复检测 → 记录清单 → gin 注册。
//
// prefix 是归一化前缀，mws 是路由级中间件（PWith/GWith），hdrs 是 handler 列表（语义与 1.23 线一致，
// 允许把 gin.HandlerFunc 塞进 hdrs 当路由级中间件）。
func (svr *Router) register(grp *gin.RouterGroup, prefix, method, action string, mws []gin.HandlerFunc, hdrs ...interface{}) {
	if svr.l != nil {
		panic(fmt.Sprintf("server: route registration must happen before Run: %s %s", method, joinRoutePath(prefix, action)))
	}
	for i, mw := range mws {
		if mw == nil {
			panic(fmt.Sprintf("server: route middleware[%d] for %q must not be nil", i, action))
		}
	}
	fullPath := joinRoutePath(prefix, action)
	keys := routeKeys(method, fullPath)
	source := registrationSource()
	for _, key := range keys {
		if registeredBy, ok := svr.routes[key]; ok {
			panic(fmt.Sprintf("server: duplicate route %q, already registered by %s, re-registering from %s", key, registeredBy, source))
		}
	}
	for _, key := range keys {
		svr.routes[key] = source
	}
	svr.routeInfos = append(svr.routeInfos, RouteInfo{
		Method: method,
		Path:   fullPath,
		Prefix: prefix,
		Source: source,
	})

	handlers := make([]gin.HandlerFunc, 0, len(mws)+len(hdrs))
	handlers = append(handlers, mws...)
	handlers = append(handlers, adaptHandlers(hdrs...)...)

	switch method {
	case http.MethodGet:
		grp.GET(action, handlers...)
	case http.MethodPost:
		grp.POST(action, handlers...)
	case methodAny:
		grp.Any(action, handlers...)
	default:
		panic(fmt.Sprintf("server: unsupported route method %q", method))
	}
}

// routeKeys 返回一条注册在重复检测索引里的键：普通方法一个键，Any 展开成 gin 的 9 个方法键。
//
// 这样 Any 与 P/G 混用同 path 时也能在启动期 panic 并给出双方注册来源（gin 自己只会报
// handlers are already registered，没有来源信息）。
func routeKeys(method, fullPath string) []string {
	if method != methodAny {
		return []string{method + " " + fullPath}
	}
	keys := make([]string, 0, len(anyRouteMethods))
	for _, anyMethod := range anyRouteMethods {
		keys = append(keys, anyMethod+" "+fullPath)
	}
	return keys
}

// adaptHandlers 把 hdrs 里的四种 handler 形态与 gin.HandlerFunc 适配成 gin 处理器链。
//
// 类型分派顺序与 1.23 线保持一致：func(*gin.Context) → gin.HandlerFunc →
// func(http.ResponseWriter, *http.Request) → http.HandlerFunc → http.Handler → transAPI 反射适配。
func adaptHandlers(hdrs ...interface{}) []gin.HandlerFunc {
	ginhdrs := make([]gin.HandlerFunc, 0, len(hdrs))
	for _, hdr := range hdrs {
		if hdr == nil {
			panic("server: handler must not be nil")
		}
		if hf, ok := hdr.(func(*gin.Context)); ok {
			ginhdrs = append(ginhdrs, hf)
		} else if hf, ok := hdr.(gin.HandlerFunc); ok {
			ginhdrs = append(ginhdrs, hf)
		} else if hf, ok := hdr.(func(http.ResponseWriter, *http.Request)); ok {
			ginhdrs = append(ginhdrs, transHttp(hf))
		} else if hf, ok := hdr.(http.HandlerFunc); ok {
			ginhdrs = append(ginhdrs, transHttp(hf))
		} else if hf, ok := hdr.(http.Handler); ok {
			ginhdrs = append(ginhdrs, transHttpHdr(hf))
		} else {
			ginhdrs = append(ginhdrs, transAPI(hdr))
		}
	}
	return ginhdrs
}

// normalizePrefix 归一化前缀：去空白、补前导斜杠、去尾部斜杠，并做 path.Clean。
//
// "/v2"、"/v2/"、"v2" 归一到 "/v2"；"/v3//x" 归一到 "/v3/x"；"/v3/../v4" 归一到 "/v4"
// （与 gin 的 BasePath 口径一致，避免 Scope 查表与真实 fullPath 分叉）；根路径退化为 "/"；
// 空串返回 ""（由调用方决定 panic）。
func normalizePrefix(prefix string) string {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return ""
	}
	trimmed := strings.Trim(prefix, "/")
	for _, segment := range strings.Split(trimmed, "/") {
		if segment == "." || segment == ".." {
			panic(fmt.Sprintf("server: prefix %q must not contain %q segment", prefix, segment))
		}
	}
	return path.Clean("/" + trimmed)
}

// joinRoutePath 拼接前缀与注册路径，语义与 gin 的 joinPaths 等价。
//
// 重复检测的键必须与 gin 实际注册的 fullPath 一致，所以既不能用字符串拼接（会拼出 "//"），
// 也不能裸用 path.Join（会吃掉尾斜杠）。等价性依赖 prefix 已被 normalizePrefix 做 path.Clean。
func joinRoutePath(prefix, action string) string {
	if action == "" {
		return prefix
	}
	finalPath := path.Join(prefix, action)
	if strings.HasSuffix(action, "/") && !strings.HasSuffix(finalPath, "/") {
		return finalPath + "/"
	}
	return finalPath
}

// registrationSource 返回注册动作的外部调用点（"file:line"）。
//
// 逐帧跳过本包内非 _test.go 的帧，因此作用域投递的多层调用最终落回业务注册代码；
// 包内单测文件算外部帧，便于断言重复注册的两个来源不同。
func registrationSource() string {
	pcs := make([]uintptr, 32)
	n := runtime.Callers(2, pcs)
	frames := runtime.CallersFrames(pcs[:n])
	for {
		frame, more := frames.Next()
		if !isServerInternalFrame(frame) {
			return fmt.Sprintf("%s:%d", frame.File, frame.Line)
		}
		if !more {
			break
		}
	}
	return "unknown"
}

// serverPackageDir 是本包源码所在目录（用于判断栈帧是否属于本包，避免硬编码模块路径）。
var serverPackageDir = func() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Dir(file)
}()

// isServerInternalFrame 判断栈帧是否属于本包的生产代码。
func isServerInternalFrame(frame runtime.Frame) bool {
	if !strings.HasPrefix(frame.File, serverPackageDir+string(os.PathSeparator)) {
		return false
	}
	return !strings.HasSuffix(frame.File, "_test.go")
}
