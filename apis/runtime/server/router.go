package server

import (
	"fmt"
	"net"
	"net/http"
	"sort"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/lifecycle"
	"github.com/ygpkg/yg-go/logs"
)

// PrefixSpec 声明一个前缀，以及只作用于该前缀的中间件。
//
// 前缀会被归一化（去空白、补前导斜杠、去尾部斜杠）后作为精确集合使用；同一前缀重复声明会去重，
// 中间件按声明顺序叠加。前缀级中间件排在引擎级中间件链之后、路由级中间件之前。
//
// 它只是「声明输入」，不是可注册的句柄：真正用于注册的是查表得到的 RouterGroup。
type PrefixSpec struct {
	// Prefix 前缀，如 "/v3"、"v3/"、"v4"，归一化后形如 "/v3"。
	Prefix string
	// Middlewares 该前缀下所有路由共享的中间件。
	Middlewares []gin.HandlerFunc
}

// RouteInfo 描述一条已注册的业务路由。
//
// Path 是完整注册路径（形如 "/v3/user/login"），与 gin 实际注册的 fullPath 一致；
// Source 是注册发生的位置（"file:line"），重复注册时用于定位两个来源。
// 直接挂在 GinEngine() 上的探针、NoRoute、静态文件不属于路由组模型，不会出现在 Routes() 里。
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
// Router 只持有「声明」与「装配结果」：前缀清单、前缀规格、gin 分组、清单索引与中间件链配置。
// 前缀集合与中间件链都在 NewRouter 内定死，之后不再变化，因此请求期无需加锁，也不存在
// 「后挂中间件对已有路由不生效」的时序陷阱——但注册动作仍必须发生在 Run 之前（见包文档）。
//
// Router 自身不实现 Registrar：投递入口是 RouterGroup（单前缀）与 AllGroups（全前缀广播）。
type Router struct {
	eng *gin.Engine
	// l 是 Run 传入的 listener；非 nil 表示服务已启动，之后禁止再注册路由。
	l  net.Listener
	lc *lifecycle.LifeCycle

	// prefixes 归一化、去重、按声明顺序排列的前缀清单（精确集合）。
	prefixes []string
	// prefixSpecs 归一化前缀 → 该前缀的规格（含前缀级中间件）；key 与 prefixes 一一对应。
	prefixSpecs map[string]*PrefixSpec
	// ginGroups 归一化前缀 → 构造期建好的 gin 分组；与 RouterGroup（本包的投递入口）不是同一层概念。
	ginGroups map[string]*gin.RouterGroup

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
// 未在声明中出现的前缀无法用于注册，Router.RouterGroup 会 panic。
func WithPrefixes(prefixes ...string) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		for _, prefix := range prefixes {
			svr.declarePrefix(prefix, nil)
		}
	}
}

// WithPrefix 声明一个前缀，并携带只作用于该前缀的中间件。
func WithPrefix(prefix string, mws ...gin.HandlerFunc) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		svr.declarePrefix(prefix, mws)
	}
}

// WithPrefixSpecs 批量声明前缀及其中间件。
func WithPrefixSpecs(specs ...PrefixSpec) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		for _, spec := range specs {
			svr.declarePrefix(spec.Prefix, spec.Middlewares)
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
		prefixSpecs:        make(map[string]*PrefixSpec),
		ginGroups:          make(map[string]*gin.RouterGroup),
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
		spec = &PrefixSpec{Prefix: normalized}
		svr.prefixSpecs[normalized] = spec
		svr.prefixes = append(svr.prefixes, normalized)
	}
	spec.Middlewares = append(spec.Middlewares, mws...)
}

// assemble 在 NewRouter 内一次性完成装配：中间件链、NoRoute、每个前缀的分组。
func (svr *Router) assemble() {
	if len(svr.prefixes) == 0 {
		panic("server: no prefix declared, use WithPrefixes or WithPrefix")
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
			svr.ginGroups[prefix] = svr.eng.Group(prefix, spec.Middlewares...)
			continue
		}
		svr.ginGroups[prefix] = svr.eng.Group(prefix)
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
