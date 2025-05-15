package server

import (
	"net"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/ygpkg/yg-go/apis/runtime/auth"
	"github.com/ygpkg/yg-go/apis/runtime/middleware"
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/lifecycle"
	"github.com/ygpkg/yg-go/logs"
)

const (
	PrefixAPIV3 = "/v3/"

	PrefixAPIDefault = "/v4/"
)

type MethodFunc func(relativePath string, handlers ...gin.HandlerFunc) gin.IRoutes

// Router .
type Router struct {
	eng    *gin.Engine
	l      net.Listener
	lc     *lifecycle.LifeCycle
	Prefix string
	pgr    *gin.RouterGroup

	routerMap map[string]interface{}

	*authInjectors
}

// NewRouter .
func NewRouter(apiPrefix string) *Router {
	if apiPrefix == "" {
		apiPrefix = PrefixAPIV3
		logs.Warnf("apiPrefix is empty, use default: %v", apiPrefix)
	}
	svr := &Router{
		eng:       gin.New(),
		lc:        lifecycle.Std(),
		Prefix:    apiPrefix,
		routerMap: map[string]interface{}{},
		authInjectors: &authInjectors{
			injectors: map[string]auth.InjectorFunc{},
			defaultInjector: func(ctx *gin.Context, ls *auth.LoginStatus) (err error) {
				return nil
			},
		},
	}
	if config.Conf().MainConf.Env != "test" {
		gin.SetMode(gin.ReleaseMode)
	}

	svr.router()

	svr.pgr = svr.eng.Group(apiPrefix)

	return svr
}

// Run .
func (svr *Router) Run(l net.Listener) error {
	svr.l = l

	go func() {
		if err := http.Serve(l, svr.eng); err != nil {
			logs.Errorf("http.Serve error: %v", err)
		}
		svr.lc.Exit()
	}()
	return nil
}

// GinEngine .
func (svr *Router) GinEngine() *gin.Engine {
	return svr.eng
}

func (svr *Router) router() {
	svr.eng.Use(middleware.CORS())
	svr.eng.Use(middleware.CustomerHeader())
	svr.eng.Use(middleware.Logger())
	svr.eng.Use(middleware.Recovery())
	svr.eng.Use(middleware.LoginStatus())
	svr.eng.Use(svr.Inject)

	svr.eng.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "The incorrect API route.")
	})
}

// Any .
func (svr *Router) Any(action string, hdrs ...interface{}) {
	svr.routerMap[action] = nil
	P(svr.pgr.Any, action, hdrs...)
}

// P .
func (svr *Router) P(action string, hdrs ...interface{}) {
	svr.routerMap[action] = nil
	P(svr.pgr.POST, action, hdrs...)
}

// G .
func (svr *Router) G(action string, hdrs ...interface{}) {
	svr.routerMap[action] = nil
	P(svr.pgr.GET, action, hdrs...)
}

// ListAllRouters 列出所有路由
func (svr *Router) ListAllRouters() {
	rts := svr.eng.Routes()
	for _, rt := range rts {
		if strings.HasPrefix(rt.Path, PrefixAPIDefault) {
			cmd := strings.TrimPrefix(rt.Path, PrefixAPIDefault)
			logs.Infof("%v", cmd)
		}
	}
}

func (svr *Router) HandleDoc(model string) {
	svr.pgr.GET(model+".docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}

// P .
func P(mf MethodFunc, action string, hdrs ...interface{}) {
	ginhdrs := make([]gin.HandlerFunc, 0, len(hdrs))
	for _, hdr := range hdrs {
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
	mf(action, ginhdrs...)
}

// PRequireLogin .
func (svr *Router) PRequireLogin(action string, hdrs ...interface{}) {
	newhdrs := append([]interface{}{middleware.AuthMiddleWare}, hdrs...)
	svr.P(action, newhdrs...)
}

// PRequireEmployee .
func (svr *Router) PRequireEmployee(action string, hdrs ...interface{}) {
	newhdrs := append([]interface{}{middleware.AuthMiddleWareEmployee}, hdrs...)
	svr.P(action, newhdrs...)
}

// GRequireLogin .
func (svr *Router) GRequireLogin(action string, hdrs ...interface{}) {
	newhdrs := append([]interface{}{middleware.AuthMiddleWare}, hdrs...)
	svr.G(action, newhdrs...)
}
