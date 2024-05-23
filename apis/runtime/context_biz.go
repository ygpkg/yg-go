package runtime

import (
	"github.com/gin-gonic/gin"

	"github.com/ygpkg/yg-go/apis/constants"
)

func CompanyID(ctx *gin.Context) uint {
	return LoginStatus(ctx).GetID(constants.CtxKeyCompanyID)
}

func EmployeeID(ctx *gin.Context) uint {
	return LoginStatus(ctx).GetID(constants.CtxKeyDcEmployeeID)
}

func UserID(ctx *gin.Context) uint {
	return LoginStatus(ctx).GetID(constants.CtxKeyUserID)
}
