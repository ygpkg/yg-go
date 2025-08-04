package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/settings"
)

func LicenseCheck(whitelist ...string) gin.HandlerFunc {
	return func(ctx *gin.Context) {
		requestPath := ctx.Request.URL.Path

		// 判断是否匹配白名单路径
		for _, skip := range whitelist {
			if strings.Contains(requestPath, skip) {
				logs.InfoContextf(ctx, "[LicenseCheck] 跳过 license 校验, path: %s 命中白名单: %s", requestPath, skip)
				ctx.Next()
				return
			}
		}

		license, getLicenseErr := settings.GetValue("license", "enable")
		if getLicenseErr != nil {
			logs.ErrorContextf(ctx, "[LicenseCheck] get license err: %s", getLicenseErr)
			ctx.AbortWithStatusJSON(http.StatusOK, gin.H{
				"code":    http.StatusBadRequest,
				"message": "license auth failed",
			})
			return
		}
		if license == "" {
			logs.ErrorContextf(ctx, "[LicenseCheck] license is empty")
			ctx.AbortWithStatusJSON(http.StatusOK, gin.H{
				"code":    http.StatusBadRequest,
				"message": "license is not found",
			})
			return
		}

		// 转为时间戳
		licenseInt, parseErr := strconv.ParseInt(license, 10, 64)

		if parseErr != nil {
			logs.ErrorContextf(ctx, "[LicenseCheck] parse license expire time fail, err: %s", parseErr)
			ctx.AbortWithStatusJSON(http.StatusOK, gin.H{
				"code":    http.StatusBadRequest,
				"message": "invalid license",
			})
			return
		}
		// 判断是否过期
		expireTime := time.UnixMicro(licenseInt)
		if time.Now().After(expireTime) {
			logs.ErrorContextf(ctx, "[LicenseCheck] license 已过期, 过期时间: %s, 当前时间: %s", expireTime.Format(time.DateTime), time.Now().Format(time.DateTime))
			ctx.AbortWithStatusJSON(http.StatusOK, gin.H{
				"code":    http.StatusBadRequest,
				"message": "license expired",
			})
			return
		}
		ctx.Next()
	}
}
