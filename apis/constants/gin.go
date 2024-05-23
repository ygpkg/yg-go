package constants

const (
	CtxKeyRequestID = "reqid"
	CtxKeyLogger    = "logger"

	CtxKeyPlatform = "platform"
	CtxKeyUserRole = "userrole"

	CtxKeyLoginStatus = "loginstatus"
)

const (
	CtxKeyAccountID          = "accountid"          // 用户端登录账户
	CtxKeyUserID             = "userid"             // 用户端家庭成员
	CtxKeyCompanyID          = "companyid"          // 大客户企业
	CtxKeyDcEmployeeID       = "dcemployeeid"       // 大客户企业员工
	CtxKeyOperatorID         = "operatorid"         // 运营端员工
	CtxKeyHealthSpecialistID = "healthspecialistid" // 健康师
)
