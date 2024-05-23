package server

import (
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
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
	var (
		authstr = ctx.Request.Header.Get("Authorization")
		ls      = &auth.LoginStatus{
			State: auth.StateNil,
			Role:  auth.RoleNil,
		}
	)
	defer ctx.Set(constants.CtxKeyLoginStatus, ls)

	authstr = strings.TrimPrefix(authstr, auth.AuthBearer)
	prefix, token, found := strings.Cut(authstr, "-")
	if !found || len(prefix) > 5 {
		ls.Token = authstr
		if ai.defaultInjector != nil {
			ls.Err = ai.defaultInjector(ctx, ls)
		}
		return
	}

	if injector, ok := ai.injectors[prefix]; ok {
		ls.Token = token
		ls.Err = injector(ctx, ls)
		return
	}

	ls.Token = token
	if ai.defaultInjector != nil {
		ls.Err = ai.defaultInjector(ctx, ls)
	}

	return
}
