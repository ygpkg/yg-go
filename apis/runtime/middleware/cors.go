package middleware

import (
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS 跨域
func CORS() gin.HandlerFunc {
	return cors.New(cors.Config{
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
		},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	})
}
