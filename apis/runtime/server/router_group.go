package server

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Registrar 是模块路由函数依赖的唯一注册接口。
//
// 它由 RouterGroup（单前缀投递）与 AllGroups（全前缀广播）共同实现：模块路由函数只接收 Registrar，
// 由装配方决定把路由投给一个前缀还是广播到所有前缀，模块内部不需要知道目标数量，也不用改签名。
// Router 自身不实现该接口——广播必须显式写成 svr.All()，从根上避免「顺手 svr.RouterGroup 就铺到全部前缀」。
//
// PRequireLogin / PRequireEmployee / GRequireLogin 是 PWith/GWith 的薄糖（与 1.23 线同名同语义），
// 只为让既有模块路由函数的调用风格保持不变；需要别的路由级中间件时用 PWith/GWith，两者可混用。
type Registrar interface {
	// P 注册 POST 路由。
	P(action string, hdrs ...interface{})
	// G 注册 GET 路由。
	G(action string, hdrs ...interface{})
	// Any 注册全部 HTTP 方法的路由。
	Any(action string, hdrs ...interface{})
	// PWith 注册 POST 路由，并携带路由级中间件（排在 handler 之前）。
	PWith(mws []gin.HandlerFunc, action string, hdrs ...interface{})
	// GWith 注册 GET 路由，并携带路由级中间件（排在 handler 之前）。
	GWith(mws []gin.HandlerFunc, action string, hdrs ...interface{})
	// PRequireLogin 要求登录态；guard 来自 AuthPolicy.Login()（默认 middleware.AuthMiddleWare）。
	PRequireLogin(action string, hdrs ...interface{})
	// PRequireEmployee 要求员工身份；guard 来自 AuthPolicy.Employee()。
	PRequireEmployee(action string, hdrs ...interface{})
	// GRequireLogin 是 GET 版登录态校验；guard 来自 AuthPolicy.Login()。
	GRequireLogin(action string, hdrs ...interface{})
	// Docs 把 swagger 与 redoc 两条文档路由挂到该路由组。
	Docs(model string)
}

// RouterGroup 是绑定到单个前缀的投递入口，它包着一个构造期建好的 gin.RouterGroup。
//
// 与 gin 的同名概念只共享「前缀 + 中间件 + 路由」这一层含义，不共享可变面：gin 的
// RouterGroup.Group 是相对父组派生、Use 可以继续追加中间件；这里没有 Group/Use/Static 等方法，
// 路由组只能按 Router.RouterGroup(prefix) 查表取得，且前缀必须在构造期声明、注册必须在 Run 之前。
//
// 它只是「归一化前缀 + 构造期已建好的分组」的纯查表结果：不持有前缀规格、不复制中间件链，
// 也不缓存注册状态。前缀级中间件在 NewRouter 内随分组一次挂好，链装配只发生在 NewRouter 一次，
// 之后不可变，因此 RouterGroup 的每次注册都不需要再碰中间件链，也不存在「后挂中间件对已有路由不生效」的时序。
type RouterGroup struct {
	svr    *Router
	prefix string
	grp    *gin.RouterGroup
}

// AllGroups 是把注册显式广播到全部已声明前缀的投递入口。
//
// 它不持有分组，也不复制注册逻辑：每次注册都按 Router.prefixes 的声明顺序，逐个复用 RouterGroup 的
// 同一条注册路径（重复检测、handler 适配、清单记录完全一致），因此「广播」与「单投递」的行为差异
// 只剩目标数量，不会出现两套语义。
type AllGroups struct {
	svr *Router
}

// RouterGroup 取得绑定到 prefix 的投递入口。
//
// prefix 先归一化（去空白、补前导斜杠、去尾部斜杠）再查前缀分组，因此 "/v3"、"v3"、"v3/" 命中同一路由组。
// 前缀集合是构造期声明的精确集合，未声明的前缀直接 panic：拼写错误必须在启动期暴露，
// 而不是懒创建出一个没人声明过的前缀，让路由悄悄挂到不存在的地方。panic 消息同时给出传入的原始
// 前缀、归一化结果与可用前缀清单，便于直接定位拼写问题。
//
// 注意这是「查表」而不是 gin 的 RouterGroup.Group 那种「派生」：不存在父子嵌套，
// 也不能对一个已经取到的 RouterGroup 再开子组，子路径请写进 action 里（如 "/user/login"）。
func (svr *Router) RouterGroup(prefix string) *RouterGroup {
	normalized := normalizePrefix(prefix)
	grp, ok := svr.ginGroups[normalized]
	if !ok {
		panic(fmt.Sprintf(
			"server: undeclared prefix %q (normalized %q), available prefixes: %v",
			prefix, normalized, svr.prefixes,
		))
	}
	return &RouterGroup{svr: svr, prefix: normalized, grp: grp}
}

// All 取得全前缀投递入口：显式广播，按声明顺序遍历 Router.prefixes。
func (svr *Router) All() *AllGroups {
	return &AllGroups{svr: svr}
}

// forEachGroup 按声明顺序把 fn 依次应用到每个前缀的 RouterGroup 上。
func (a *AllGroups) forEachGroup(fn func(group *RouterGroup)) {
	for _, prefix := range a.svr.prefixes {
		fn(a.svr.RouterGroup(prefix))
	}
}

// P 注册 POST 路由到本路由组。
func (g *RouterGroup) P(action string, hdrs ...interface{}) {
	g.svr.register(g.grp, g.prefix, http.MethodPost, action, nil, hdrs...)
}

// G 注册 GET 路由到本路由组。
func (g *RouterGroup) G(action string, hdrs ...interface{}) {
	g.svr.register(g.grp, g.prefix, http.MethodGet, action, nil, hdrs...)
}

// Any 注册全部 HTTP 方法的路由到本路由组。
func (g *RouterGroup) Any(action string, hdrs ...interface{}) {
	g.svr.register(g.grp, g.prefix, methodAny, action, nil, hdrs...)
}

// PWith 注册 POST 路由到本路由组，mws 作为路由级中间件排在 handler 之前。
func (g *RouterGroup) PWith(mws []gin.HandlerFunc, action string, hdrs ...interface{}) {
	g.svr.register(g.grp, g.prefix, http.MethodPost, action, mws, hdrs...)
}

// GWith 注册 GET 路由到本路由组，mws 作为路由级中间件排在 handler 之前。
func (g *RouterGroup) GWith(mws []gin.HandlerFunc, action string, hdrs ...interface{}) {
	g.svr.register(g.grp, g.prefix, http.MethodGet, action, mws, hdrs...)
}

// PRequireLogin 注册要求登录态的 POST 路由。
//
// 语义等价于「PWith + 默认策略的 middleware.AuthMiddleWare」；策略可被
// WithAuthPolicy / WithPrefixAuthPolicy 替换（此时 guard 由自定义 AuthPolicy 决定）。
// 保留与 1.23 线同名同语义的薄糖：既有模块路由函数不必改调用风格。它与 PWith 可以混用，
// 需要登录态之外的准入条件（或别的路由级中间件）时直接用 PWith。
func (g *RouterGroup) PRequireLogin(action string, hdrs ...interface{}) {
	g.PWith([]gin.HandlerFunc{g.loginGuard()}, action, hdrs...)
}

// PRequireEmployee 注册要求员工身份的 POST 路由（guard 来自 AuthPolicy.Employee()）。
func (g *RouterGroup) PRequireEmployee(action string, hdrs ...interface{}) {
	g.PWith([]gin.HandlerFunc{g.employeeGuard()}, action, hdrs...)
}

// GRequireLogin 注册要求登录态的 GET 路由（guard 来自 AuthPolicy.Login()）。
func (g *RouterGroup) GRequireLogin(action string, hdrs ...interface{}) {
	g.GWith([]gin.HandlerFunc{g.loginGuard()}, action, hdrs...)
}

// loginGuard 返回本路由组生效的登录态 guard；策略返回 nil 时在注册期 panic，而不是
// 把 nil 带进 handler 链、让每个请求 500。
func (g *RouterGroup) loginGuard() gin.HandlerFunc {
	guard := g.svr.authPolicyFor(g.prefix).Login()
	if guard == nil {
		panic(fmt.Sprintf("server: PRequireLogin/GRequireLogin: AuthPolicy.Login() returned nil for prefix %q", g.prefix))
	}
	return guard
}

// employeeGuard 返回本路由组生效的员工身份 guard；策略返回 nil 时在注册期 panic。
func (g *RouterGroup) employeeGuard() gin.HandlerFunc {
	guard := g.svr.authPolicyFor(g.prefix).Employee()
	if guard == nil {
		panic(fmt.Sprintf("server: PRequireEmployee: AuthPolicy.Employee() returned nil for prefix %q", g.prefix))
	}
	return guard
}

// Docs 把 swagger（model+".docs/*any"）与 redoc（model+".redocs"）两条 GET 路由挂到本路由组。
//
// 两条路由都走 svr.register，因此同样计入重复检测与 Routes() 清单。
func (g *RouterGroup) Docs(model string) {
	// swaggerFiles.Handler 是包级单例，WrapHandler 会把首次请求路径写进它的 Prefix；
	// 多个前缀/多个 model 共用同一实例时，后访问的文档页会因 Prefix 不匹配导致静态资源 404。
	// 这里给每个 Docs 挂载点复制一份 handler，保证 Prefix 相互独立。
	swaggerHandler := *swaggerFiles.Handler
	g.svr.register(g.grp, g.prefix, http.MethodGet, model+".docs/*any",
		nil, ginSwagger.WrapHandler(&swaggerHandler, ginSwagger.InstanceName(model)))

	// doc.json 的 URL 必须与上面那条 wildcard 路由实际注册的完整路径一致（"/v3/<model>.docs/doc.json"）。
	// 前缀已归一化为无尾斜杠的 "/v3"，直接拼 grp.BasePath() 会得到 "/v3<model>.docs/doc.json"，
	// 所以这里复用与注册相同的拼接规则。
	g.svr.register(g.grp, g.prefix, http.MethodGet, model+".redocs",
		nil, ReDocHandler(model, joinRoutePath(g.prefix, model+".docs/doc.json")))
}

// P 广播注册 POST 路由到全部已声明前缀（按声明顺序）。
func (a *AllGroups) P(action string, hdrs ...interface{}) {
	a.forEachGroup(func(group *RouterGroup) { group.P(action, hdrs...) })
}

// G 广播注册 GET 路由到全部已声明前缀（按声明顺序）。
func (a *AllGroups) G(action string, hdrs ...interface{}) {
	a.forEachGroup(func(group *RouterGroup) { group.G(action, hdrs...) })
}

// Any 广播注册全部 HTTP 方法的路由到全部已声明前缀（按声明顺序）。
func (a *AllGroups) Any(action string, hdrs ...interface{}) {
	a.forEachGroup(func(group *RouterGroup) { group.Any(action, hdrs...) })
}

// PWith 广播注册带路由级中间件的 POST 路由到全部已声明前缀（按声明顺序）。
func (a *AllGroups) PWith(mws []gin.HandlerFunc, action string, hdrs ...interface{}) {
	a.forEachGroup(func(group *RouterGroup) { group.PWith(mws, action, hdrs...) })
}

// GWith 广播注册带路由级中间件的 GET 路由到全部已声明前缀（按声明顺序）。
func (a *AllGroups) GWith(mws []gin.HandlerFunc, action string, hdrs ...interface{}) {
	a.forEachGroup(func(group *RouterGroup) { group.GWith(mws, action, hdrs...) })
}

// PRequireLogin 把「要求登录态的 POST 路由」广播到全部已声明前缀（按声明顺序）。
func (a *AllGroups) PRequireLogin(action string, hdrs ...interface{}) {
	a.forEachGroup(func(group *RouterGroup) { group.PRequireLogin(action, hdrs...) })
}

// PRequireEmployee 把「要求员工身份的 POST 路由」广播到全部已声明前缀（按声明顺序）。
func (a *AllGroups) PRequireEmployee(action string, hdrs ...interface{}) {
	a.forEachGroup(func(group *RouterGroup) { group.PRequireEmployee(action, hdrs...) })
}

// GRequireLogin 把「要求登录态的 GET 路由」广播到全部已声明前缀（按声明顺序）。
func (a *AllGroups) GRequireLogin(action string, hdrs ...interface{}) {
	a.forEachGroup(func(group *RouterGroup) { group.GRequireLogin(action, hdrs...) })
}

// Docs 把 swagger 与 redoc 路由广播挂到全部已声明前缀（按声明顺序）。
func (a *AllGroups) Docs(model string) {
	a.forEachGroup(func(group *RouterGroup) { group.Docs(model) })
}

// 编译期断言：两个投递入口都必须完整实现 Registrar，模块路由函数才能无差别地接收它们。
var (
	_ Registrar = (*RouterGroup)(nil)
	_ Registrar = (*AllGroups)(nil)
)
