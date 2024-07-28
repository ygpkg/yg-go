package runtime

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/apiobj"
	"github.com/ygpkg/yg-go/apis/errcode"
	"github.com/ygpkg/yg-go/logs"
)

// ResponseMessage 返回消息
func ResponseMessage(ctx *gin.Context, msgCode uint32, msg string) {
	m := apiobj.BaseResponse{Code: uint32(msgCode), Message: msg}
	logs.WarnContextf(ctx, "badrequest code %v, message: %s", m.Code, m.Message)
	body, _ := json.Marshal(m)
	ctx.Writer.Write(body)
	ctx.Abort()
}

// Success 成功
func Success(ctx *gin.Context, msgs ...interface{}) {
	ctx.Writer.WriteHeader(http.StatusOK)
	ResponseMessage(ctx, 0, formatMessage(msgs))
}

// BadRequest 参数错误
func BadRequest(ctx *gin.Context, msgs ...interface{}) {
	ctx.Writer.WriteHeader(http.StatusBadRequest)
	ResponseMessage(ctx, errcode.ErrCode_BadRequest, formatMessage(msgs))
}

// BadRequestWithCode 参数错误
func BadRequestWithCode(ctx *gin.Context, code int, msgs ...interface{}) {
	ctx.Writer.WriteHeader(http.StatusBadRequest)
	ResponseMessage(ctx, errcode.ErrCode_BadRequest, formatMessage(msgs))
}

// InternalError 服务器内部错误
func InternalError(ctx *gin.Context, msgs ...interface{}) {
	ctx.Writer.WriteHeader(http.StatusInternalServerError)
	ResponseMessage(ctx, errcode.ErrCode_InternalError, formatMessage(msgs))
}

// InternalErrorWithCode 服务器内部错误
func InternalErrorWithCode(ctx *gin.Context, code int, msgs ...interface{}) {
	ctx.Writer.WriteHeader(http.StatusInternalServerError)
	ResponseMessage(ctx, errcode.ErrCode_InternalError, formatMessage(msgs))
}

func formatMessage(msgs []interface{}) string {
	if len(msgs) == 0 {
		return ""
	}
	if len(msgs) == 1 {
		return fmt.Sprint(msgs[0])
	}
	if format, ok := msgs[0].(string); ok {
		return fmt.Sprintf(format, msgs[1:]...)
	}
	return fmt.Sprint(msgs...)
}
