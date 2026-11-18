package server

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"

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
	eng      *gin.Engine
	l        net.Listener
	lc       *lifecycle.LifeCycle
	Prefix   string
	prefixes []string
	pgr      *gin.RouterGroup

	routerMap   map[string]interface{}
	routeGroups map[string]*gin.RouterGroup

	deployMode string

	// loggerOpts 由 WithLoggerOptions 收集，在首个请求时被懒构造进中间件
	// （opts 是在 router() 之后才 apply 的，那时两件中间件已经装进引擎）。
	loggerOpts []middleware.LoggerOption

	*authInjectors
}

type RouterOption func(*Router)

func WithPrefixes(prefixes []string) RouterOption {
	return func(svr *Router) {
		if svr == nil {
			return
		}

		uniquePrefixes := make(map[string]struct{})

		if svr.Prefix != "" {
			uniquePrefixes[svr.Prefix] = struct{}{}
		}

		for _, p := range prefixes {
			if p != "" {
				uniquePrefixes[p] = struct{}{}
			}
		}

		svr.prefixes = make([]string, 0, len(uniquePrefixes))
		for p := range uniquePrefixes {
			svr.prefixes = append(svr.prefixes, p)
		}
	}
}

func WithDeployMode(deployMode string) RouterOption {
	return func(svr *Router) {
		svr.deployMode = deployMode
	}
}

// WithLoggerOptions 配置框架自带的访问日志与字段写入中间件。
//
// 一个选项都不传时，日志输出与历史版本逐字一致（不新增字段、不改响应头）。
// 需要身份字段（companyid/uin）、业务维度或 X-Request-Id 回写时在这里显式打开，
// app 侧不必再自己挂日志中间件。
func WithLoggerOptions(opts ...middleware.LoggerOption) RouterOption {
	return func(svr *Router) {
		svr.loggerOpts = append(svr.loggerOpts, opts...)
	}
}

func WithMiddleware(middleware ...gin.HandlerFunc) RouterOption {
	return func(svr *Router) {
		svr.eng.Use(middleware...)
	}
}

// NewRouter .
func NewRouter(apiPrefix string, opts ...RouterOption) *Router {
	if apiPrefix == "" {
		logs.Warnf("apiPrefix is empty, use default: %v", apiPrefix)
		apiPrefix = PrefixAPIV3
	}
	svr := &Router{
		eng:         gin.New(),
		lc:          lifecycle.Std(),
		Prefix:      apiPrefix,
		routerMap:   map[string]interface{}{},
		routeGroups: map[string]*gin.RouterGroup{},
		authInjectors: &authInjectors{
			injectors: map[string]auth.InjectorFunc{},
			defaultInjector: func(ctx *gin.Context, ls *auth.LoginStatus) (err error) {
				return nil
			},
		},
	}
	svr.router()

	for _, opt := range opts {
		opt(svr)
	}

	if len(svr.prefixes) == 0 {
		svr.prefixes = []string{svr.Prefix}
	}

	if config.Conf().MainConf.Env != "test" {
		gin.SetMode(gin.ReleaseMode)
	}

	for _, p := range svr.prefixes {
		svr.routeGroups[p] = svr.eng.Group(p)
	}

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
	svr.eng.Use(svr.accessLogger())
	svr.eng.Use(middleware.Recovery())
	svr.eng.Use(middleware.LoginStatus())
	svr.eng.Use(middleware.AcceptLanguage())
	svr.eng.Use(svr.Inject)
	// 字段写入点：必须在 Inject 之后（idmap 那时才填好），且早于路由处理（业务日志才带得上）。
	svr.eng.Use(svr.contextFields())

	svr.eng.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "The incorrect API route.")
	})
}

// accessLogger 返回访问日志中间件。
//
// 为什么要懒构造：`NewRouter` 先 `router()` 装框架中间件、再 apply opts，而
// `WithLoggerOptions` 是 opts 之一——装机时选项还没到。首个请求时构造一次即可，
// 之后复用同一个 handler（sync.Once，无锁热路径）。
func (svr *Router) accessLogger() gin.HandlerFunc {
	var (
		once sync.Once
		h    gin.HandlerFunc
	)
	return func(ctx *gin.Context) {
		once.Do(func() { h = middleware.LoggerWithOptions(svr.loggerOptions()...) })
		h(ctx)
	}
}

// contextFields 返回字段写入中间件，同样是懒构造。
func (svr *Router) contextFields() gin.HandlerFunc {
	var (
		once sync.Once
		h    gin.HandlerFunc
	)
	return func(ctx *gin.Context) {
		once.Do(func() { h = middleware.ContextFields(svr.loggerOptions()...) })
		h(ctx)
	}
}

// loggerOptions 合并默认白名单与调用方选项（调后方可覆盖白名单）。
func (svr *Router) loggerOptions() []middleware.LoggerOption {
	opts := make([]middleware.LoggerOption, 0, len(svr.loggerOpts)+1)
	opts = append(opts, middleware.WithWhitelist(".Ping", "metrics"))
	opts = append(opts, svr.loggerOpts...)
	return opts
}

// Any .
func (svr *Router) Any(action string, hdrs ...interface{}) {
	svr.routerMap[action] = nil
	for _, pg := range svr.routeGroups {
		P(pg.Any, action, hdrs...)
	}
	// P(svr.pgr.Any, action, hdrs...)
}

// P .
func (svr *Router) P(action string, hdrs ...interface{}) {
	svr.routerMap[action] = nil
	for _, pg := range svr.routeGroups {
		P(pg.POST, action, hdrs...)
	}
	// P(svr.pgr.POST, action, hdrs...)
}

// G .
func (svr *Router) G(action string, hdrs ...interface{}) {
	svr.routerMap[action] = nil
	for _, pg := range svr.routeGroups {
		P(pg.GET, action, hdrs...)
	}
	// P(svr.pgr.GET, action, hdrs...)
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
	for _, pg := range svr.routeGroups {
		pg.GET(model+".docs/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.InstanceName(model)))
		pg.GET(model+".redocs", ReDocHandler(model, fmt.Sprintf("%s%s.docs/doc.json", pg.BasePath(), model)))
	}
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
