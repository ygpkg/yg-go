package server

import (
	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/logs"
)

type authInjectors struct {
	injectors map[string]auth.InjectorFunc

	defaultInjector auth.InjectorFunc
}

func (ai *authInjectors) AuthInject(issuer string, injector auth.InjectorFunc) {
	if _, ok := ai.injectors[issuer]; ok {
		panic("injector issuer is already exists")
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
	ls := runtime.LoginStatus(ctx)
	if ls.State != auth.StateSucc {
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
