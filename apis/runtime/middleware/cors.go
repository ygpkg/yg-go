package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORSOption 是 CORS 中间件的选项。
//
// 选项按「实际需要扩展的点」拆开，而不是整体替换 cors.Config：来源策略、允许的方法/头、
// 暴露的响应头、凭证、预检缓存。确需完整控制（AllowWildcard / AllowPrivateNetwork /
// AllowOriginWithContextFunc 等）时，直接用 cors.New(cfg) 替换内置条目：
//
//	server.WithMiddlewareImpl(server.CORS, cors.New(myConfig))
type CORSOption func(*cors.Config)

// WithCORSAllowOrigins 指定允许的来源（覆盖默认的「允许所有来源」）。
//
// 传空表示不做修改（默认仍是「允许所有来源」）；需要按请求动态判断（多环境域名、租户子域、
// preview 域名）时用 WithCORSAllowOriginFunc。注意 gin-contrib/cors 的 AllowWildcard 没有暴露：
// 传 "https://*.example.com" 这类含 * 的来源会通过校验但实际不匹配（全部 403），
// 通配来源请用 WithCORSAllowOriginFunc 或整条替换 CORS 实现。
func WithCORSAllowOrigins(origins ...string) CORSOption {
	return func(cfg *cors.Config) {
		if len(origins) == 0 {
			cfg.AllowAllOrigins = true
			cfg.AllowOrigins = nil
			cfg.AllowOriginFunc = nil
			return
		}
		cfg.AllowAllOrigins = false
		cfg.AllowOrigins = append([]string{}, origins...)
		cfg.AllowOriginFunc = nil
	}
}

// WithCORSAllowOriginFunc 用函数动态判断来源是否允许（与 WithCORSAllowOrigins 互斥，后者会被清空）。
func WithCORSAllowOriginFunc(verify func(origin string) bool) CORSOption {
	return func(cfg *cors.Config) {
		if verify == nil {
			panic("middleware: WithCORSAllowOriginFunc: verify must not be nil")
		}
		cfg.AllowAllOrigins = false
		cfg.AllowOrigins = nil
		cfg.AllowOriginFunc = verify
	}
}

// WithCORSAllowMethods 覆盖允许的跨域方法（默认 GET/POST/PUT/DELETE/PATCH）；传空保持默认。
func WithCORSAllowMethods(methods ...string) CORSOption {
	return func(cfg *cors.Config) {
		if len(methods) == 0 {
			return
		}
		cfg.AllowMethods = append([]string{}, methods...)
	}
}

// WithCORSAllowHeaders 在默认允许请求头的基础上**追加**自定义头（如 X-Tenant-Id、X-Client-Version）。
//
// 默认头名单已包含 Content-Type / Authorization / Env / App-Version / App-Type 等常用头；
// 注意 gin-contrib/cors 不会把 AllowHeaders 里的 "*" 解释成通配（它会被原样写进
// Access-Control-Allow-Headers），要允许任意请求头请用 WithMiddlewareImpl 整条替换 CORS。
func WithCORSAllowHeaders(headers ...string) CORSOption {
	return func(cfg *cors.Config) {
		cfg.AllowHeaders = append(cfg.AllowHeaders, headers...)
	}
}

// WithCORSExposeHeaders 追加暴露给浏览器的响应头（如 X-Request-Id、分页头）。
// 默认只暴露 Content-Length。
func WithCORSExposeHeaders(headers ...string) CORSOption {
	return func(cfg *cors.Config) {
		cfg.ExposeHeaders = append(cfg.ExposeHeaders, headers...)
	}
}

// WithCORSAllowCredentials 设置是否允许携带凭证（默认允许）。
func WithCORSAllowCredentials(allow bool) CORSOption {
	return func(cfg *cors.Config) {
		cfg.AllowCredentials = allow
	}
}

// WithCORSMaxAge 设置预检结果的缓存时间（默认 12h）。
func WithCORSMaxAge(age time.Duration) CORSOption {
	return func(cfg *cors.Config) {
		cfg.MaxAge = age
	}
}

// CORS 跨域。默认配置等价于 1.23 线的固定配置（允许所有来源、固定请求头名单、12h 预检缓存），
// 需要扩展时传选项：
//
//	middleware.CORS(
//	    middleware.WithCORSAllowOrigins("https://app.example.com", "https://admin.example.com"),
//	    middleware.WithCORSAllowHeaders("X-Tenant-Id"),
//	    middleware.WithCORSExposeHeaders("X-Request-Id"),
//	)
func CORS(opts ...CORSOption) gin.HandlerFunc {
	cfg := defaultCORSConfig()
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	return cors.New(cfg)
}

// defaultCORSConfig 返回默认跨域配置（与 1.23 线的 middleware.CORS() 一致）。
func defaultCORSConfig() cors.Config {
	return cors.Config{
		AllowAllOrigins: true,
		AllowMethods:    []string{"GET", "POST", "PUT", "DELETE", "PATCH"},
		AllowHeaders: []string{
			"Accept",
			"Origin",
			"Accept-Encoding",
			"Accept-Language",
			"Access-Control-Request-Headers",
			"Access-Control-Request-Method",
			"Host",
			"Proxy-Connection",
			"Referer",
			"Sec-Fetch-Mode",
			"User-Agent",
			"Content-Type",
			"Env",
			"Authorization",
			"Upgrade",
			"Connection",
			"App-Version",
			"App-Type",
		},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}
}
