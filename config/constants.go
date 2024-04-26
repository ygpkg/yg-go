package config

const (
	// PID_RPA_EMPLOYEE RPA员工端信息管理平台
	PID_EMPLOYEE = "rpa-employee"
	// PID_BACKEND RPA员工端后台
	PID_BACKEND = "rpa-backend"
	// PID_DEV 开发平台
	PID_DEV = "rpa-devops"
	// PID_EMPLOYEE_DEVICE 员工端设备
	PID_EMPLOYEE_DEVICE = "emp-device"
	// PID_CUSTOMER 客户端
	PID_CUSTOMER = "customer"
)

const (
	// APPID_ZMROBOT RPA员工端信息管理平台
	APPID_ZMROBOT = "zmrobot"
	// APPID_COMP_MANAGER RPA员工端信息管理平台
	APPID_COMP_MANAGER = "company"
	// APPID_DEVOPS 运维开发系统
	APPID_DEVOPS = "devops"
)

// GetPlatformName 获取平台名称
func GetPlatformName(pid string) string {
	switch pid {
	case PID_EMPLOYEE:
		return "员工系统"
	case PID_BACKEND:
		return "运营后台"
	case PID_DEV:
		return "开发运维系统"
	case PID_EMPLOYEE_DEVICE:
		return "员工端设备"
	default:
		return "其它"
	}
}

// GetApplicationName 获取应用名称
func GetApplicationName(appid string) string {
	switch appid {
	case APPID_ZMROBOT:
		return "挂课系统"
	case APPID_COMP_MANAGER:
		return "企业管理"
	case APPID_DEVOPS:
		return "开发运维系统"
	default:
		return "其它"
	}
}

// IsValidPlatform 判断平台是否有效
func IsValidPlatform(pid string) bool {
	switch pid {
	case PID_EMPLOYEE, PID_BACKEND, PID_DEV, PID_EMPLOYEE_DEVICE:
		return true
	default:
		return false
	}
}
