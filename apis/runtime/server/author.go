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

func (ai *authInjectors) AuthInject(name string, injector auth.InjectorFunc) {
	if _, ok := ai.injectors[name]; ok {
		panic("injector name is already exists")
	}
	logs.Infof("register auth injector: %s", name)
	ai.injectors[name] = injector
	if name == "" {
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
		ls.State = auth.StateFailed
		return
	}

	if ai.defaultInjector != nil {
		ls.Err = ai.defaultInjector(ctx, ls)
	}

	return
}
