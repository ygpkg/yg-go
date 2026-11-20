package server

import (
	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime/middleware"
)

// AuthPolicy 把「账号体系相关」的准入中间件收敛成一个可替换的接口。
//
// 三个鉴权糖按下面的映射取用：
//   - PRequireLogin / GRequireLogin → Login()
//   - PRequireEmployee            → Employee()
//
// 默认实现（DefaultAuthPolicy）就是 yg-go 内置的 middleware.AuthMiddleWare /
// middleware.AuthMiddleWareEmployee，即读 runtime.LoginStatus 的登录态判定；账号体系不同的服务
// 实现本接口后，用 WithAuthPolicy（全局）或 WithPrefixAuthPolicy（按前缀）在构造期注入，
// 路由注册侧仍然写 PRequireLogin / PRequireEmployee，调用风格不变。
//
// 约定：返回的中间件自行完成「判定 + 拒绝响应 + Abort」，放行时调用 ctx.Next()。
// 框架不在外面包统一 401 —— 不同服务的错误码与文案不同（例如「身份服务不可达」与
// 「成员资格被撤销」不能都退化成 401），把响应形态留给实现方才能表达这种差异。
// 要按单条路由追加更多判据（限流、幂等、租户校验等），继续用 PWith 或 hdrs，两者可叠加。
type AuthPolicy interface {
	// Login 返回登录态准入中间件，供 PRequireLogin / GRequireLogin 使用。
	Login() gin.HandlerFunc
	// Employee 返回员工身份准入中间件，供 PRequireEmployee 使用。
	Employee() gin.HandlerFunc
}

// DefaultAuthPolicy 返回 yg-go 内置策略。
//
// 等价于 Login = middleware.AuthMiddleWare（LoginStatus.State != StateSucc 即 401）、
// Employee = middleware.AuthMiddleWareEmployee（额外要求 Role == RoleEmployee）。
func DefaultAuthPolicy() AuthPolicy {
	return builtinAuthPolicy{}
}

// NewAuthPolicy 用两个自定义中间件快速构造策略；某一级别传 nil 时回退到内置实现。
//
// 典型用法（账号体系不经过 yg-go LoginStatus 的服务）：
//
//	svr := server.NewRouter(
//	    server.WithPrefixes("/api/v1"),
//	    server.WithoutMiddleware(server.LoginStatus), // 不再解析 yg-go token
//	    server.WithAuthPolicy(server.NewAuthPolicy(myGate, myAdminGate)),
//	)
//	svr.RouterGroup("/api/v1").PRequireLogin("file.Upload", upload)
//
// 这样 PRequireLogin 不再依赖 runtime.LoginStatus，也就不会出现 MustGet panic。
func NewAuthPolicy(login, employee gin.HandlerFunc) AuthPolicy {
	defaults := builtinAuthPolicy{}
	if login == nil {
		login = defaults.Login()
	}
	if employee == nil {
		employee = defaults.Employee()
	}
	return staticAuthPolicy{login: login, employee: employee}
}

// WithAuthPolicy 设置全局默认鉴权策略（构造期；nil 会 panic）。
//
// 与「账号体系」相关的服务级差异放这里；单个前缀要用不同体系时用 WithPrefixAuthPolicy，
// 单条路由要额外判据时用 PWith/hdrs。
func WithAuthPolicy(policy AuthPolicy) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		if policy == nil {
			panic("server: WithAuthPolicy: policy must not be nil")
		}
		svr.authPolicy = policy
	}
}

// WithPrefixAuthPolicy 为指定前缀覆盖鉴权策略（构造期；未声明前缀会 panic）。
//
// 同一 Router 内不同版本可以用不同账号体系：例如 /v3 走 yg-go 登录态、/v4 走新身份网关，
// 两个前缀各自 PRequireLogin，互不影响。未覆盖的前缀回退到 WithAuthPolicy 的全局策略。
func WithPrefixAuthPolicy(prefix string, policy AuthPolicy) RouterOption {
	return func(svr *Router) {
		svr.ensureMutable()
		if policy == nil {
			panic("server: WithPrefixAuthPolicy: policy must not be nil")
		}
		normalized := normalizePrefix(prefix)
		if normalized == "" {
			panic("server: WithPrefixAuthPolicy: prefix must not be blank")
		}
		svr.prefixAuthPolicies[normalized] = policy
	}
}

// authPolicyFor 返回某个归一化前缀实际生效的鉴权策略：前缀覆盖优先，否则全局默认。
func (svr *Router) authPolicyFor(prefix string) AuthPolicy {
	if policy, ok := svr.prefixAuthPolicies[prefix]; ok {
		return policy
	}
	if svr.authPolicy != nil {
		return svr.authPolicy
	}
	return builtinAuthPolicy{}
}

// builtinAuthPolicy 是 yg-go 内置策略，读 runtime.LoginStatus。
type builtinAuthPolicy struct{}

// Login 实现 AuthPolicy。
func (builtinAuthPolicy) Login() gin.HandlerFunc {
	return middleware.AuthMiddleWare
}

// Employee 实现 AuthPolicy。
func (builtinAuthPolicy) Employee() gin.HandlerFunc {
	return middleware.AuthMiddleWareEmployee
}

// staticAuthPolicy 是 NewAuthPolicy 的返回值：两个级别各持一个固定中间件。
type staticAuthPolicy struct {
	login    gin.HandlerFunc
	employee gin.HandlerFunc
}

// Login 实现 AuthPolicy。
func (p staticAuthPolicy) Login() gin.HandlerFunc {
	return p.login
}

// Employee 实现 AuthPolicy。
func (p staticAuthPolicy) Employee() gin.HandlerFunc {
	return p.employee
}
