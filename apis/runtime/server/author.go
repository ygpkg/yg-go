package server

import (
	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/logs"
)

type authInjectors struct {
	injectors map[string]auth.InjectorFunc

	defaultInjector auth.InjectorFunc
}

func (ai *authInjectors) AuthInject(issuer string, injector auth.InjectorFunc) {
	if _, ok := ai.injectors[issuer]; ok {
		logs.Warnf("injector issuer is already exists: %s", issuer)
	}
	logs.Infof("register auth injector: %s", issuer)
	ai.injectors[issuer] = injector
	if issuer == "" {
		ai.defaultInjector = injector
	}
}

func (ai *authInjectors) Default(injector auth.InjectorFunc) {
	ai.defaultInjector = injector
}

func (ai *authInjectors) Inject(ctx *gin.Context) {
	// 登录态中间件可能被 WithoutMiddleware(server.LoginStatus) 关掉（账号体系完全自研的服务），
	// 此时没有 LoginStatus 可读：直接跳过注入器，避免 runtime.LoginStatus 的 MustGet panic。
	// 注入器的唯一职责就是给 LoginStatus 填 idmap，没有登录态时无事可做。
	value, ok := ctx.Get(constants.CtxKeyLoginStatus)
	if !ok {
		return
	}
	ls, ok := value.(*auth.LoginStatus)
	if !ok || ls.State != auth.StateSucc {
		return
	}

	if injector, ok := ai.injectors[ls.Claim.Issuer]; ok {
		ls.Err = injector(ctx, ls)
	} else if ai.defaultInjector != nil {
		ls.Err = ai.defaultInjector(ctx, ls)
	}
	if ls.Err != nil {
		logs.Warnf("[auth][%v] auth failed, %s", ls.Claim.Uin, ls.Err)
		ls.State = auth.StateFailed
	}
}
