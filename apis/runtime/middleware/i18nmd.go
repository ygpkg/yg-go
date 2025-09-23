package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/constants"
)

// AcceptLanguage ...
func AcceptLanguage() gin.HandlerFunc {
	return func(c *gin.Context) {
		acceptLang := c.Request.Header.Get("Accept-Language")
		c.Set(constants.CtxKeyLang, acceptLang)
	}
}
