// Package server 提供 yg-go 下游服务共用的 HTTP 装配层。
//
// # 路由模型
//
// 自 v1.27 起注册入口收敛为「路由组投递」，不再有隐式广播：
//
//   - 前缀在构造期声明，精确集合：NewRouter(WithPrefixes("/v3", "/v2"))。构造参数里不再有前缀，
//     也不存在隐含的默认前缀或自动合并；需要携带前缀级中间件时用 WithPrefix / WithPrefixSpecs。
//   - 注册只发生在路由组上：Router.RouterGroup(prefix) 投递到单个前缀，Router.All() 显式广播到全部前缀。
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
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/gin-gonic/gin"
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

// register 是 RouterGroup 与 AllGroups 共用的注册路径：重复检测 → 记录清单 → gin 注册。
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
// （与 gin 的 BasePath 口径一致，避免 RouterGroup 查表与真实 fullPath 分叉）；根路径退化为 "/"；
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
// 逐帧跳过本包内非 _test.go 的帧，因此路由组投递的多层调用最终落回业务注册代码；
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
