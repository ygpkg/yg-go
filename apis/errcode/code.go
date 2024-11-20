package errcode

import "net/http"

const (
	CodeOK uint32 = 0

	ErrCode_BadRequest    = http.StatusBadRequest
	ErrCode_InternalError = http.StatusInternalServerError
	ErrCode_NotFound      = http.StatusNotFound
	ErrCode_Unauthorized  = http.StatusUnauthorized
	ErrCode_NoPermission  = http.StatusForbidden

	ErrCode_WrongUsernameOrPassword        = 10001
	ErrCode_UserStatusNotNormal            = 10002
	ErrCode_UserHasNoPlatform              = 10003
	ErrCode_NotSupportMobileForgotPassword = 10004
	ErrCode_SendVerifyCodeTooBusy          = 10005
	ErrCode_PasswordTooShort               = 10006
	ErrCode_RequireMemberLogin             = 10007 // 需要添加家庭成员并选择
)

var (
	errCodeMap = map[uint32]string{
		ErrCode_WrongUsernameOrPassword:        "用户名或密码错误",
		ErrCode_UserStatusNotNormal:            "用户状态不正常",
		ErrCode_UserHasNoPlatform:              "用户没有可用资源",
		ErrCode_NotSupportMobileForgotPassword: "暂不支持手机号找回密码",
		ErrCode_SendVerifyCodeTooBusy:          "发送验证码太过频繁",
		ErrCode_PasswordTooShort:               "密码太短",
	}
)

// GetMessage returns the error message of the error code.
func GetMessage(code uint32) string {
	return errCodeMap[code]
}
