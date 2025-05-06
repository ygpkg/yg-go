package runtime

import (
	"github.com/gin-gonic/gin"

	"github.com/ygpkg/yg-go/apis/constants"
)

// CompanyID 企业ID
func CompanyID(ctx *gin.Context) uint {
	return LoginStatus(ctx).GetID(constants.CtxKeyCompanyID)
}

// EmployeeID 员工ID
func EmployeeID(ctx *gin.Context) uint {
	return LoginStatus(ctx).GetID(constants.CtxKeyEmployeeID)
}

// Uin uin
func Uin(ctx *gin.Context) uint {
	return LoginStatus(ctx).GetID(constants.CtxKeyUin)
}

// CtxKeyAPIKeyID
func APIKeyID(ctx *gin.Context) uint {
	return LoginStatus(ctx).GetID(constants.CtxKeyAPIKeyID)
}
