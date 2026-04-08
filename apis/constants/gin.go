package constants

const (
	CtxKeyRequestID = "reqid"
	CtxKeyTraceID   = "traceid"
	CtxKeyLogger    = "logger"
	CtxKeyCode      = "code"

	CtxKeyPlatform = "platform"
	CtxKeyUserRole = "userrole"

	CtxKeyLoginStatus = "loginstatus"
)

const (
	CtxKeyCompanyID  = "companyid"
	CtxKeyEmployeeID = "employeeid" // 运营端员工
	CtxKeyUin        = "uin"
	CtxKeyAPIKeyID   = "api_key_id"
	CtxKeyLang       = "lang"
)

const (
	HeaderKeyRequestID = "X-Request-Id"
	HeaderKeyTraceID   = "X-Trace-Id"
)
