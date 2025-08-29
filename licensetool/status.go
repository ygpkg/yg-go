package licensetool

import "fmt"

// ValidationStatus 定义了许可证校验的最终状态
type ValidationStatus int

const (
	StatusValid            ValidationStatus = iota // 0: 许可证有效
	StatusInvalidFormat                            // 1: 格式错误
	StatusInvalidSignature                         // 2: 签名无效
	StatusUIDMismatch                              // 3: UID 不匹配
	StatusExpired                                  // 4: 已过期
	StatusTampered                                 // 5: 哈希链被篡改
	StatusEnvError                                 // 6: 环境错误（无法获取UID、License文件等）
	StatusInternalError                            // 7: 内部错误（如数据库、配置问题）
)

// String 使状态可读
func (s ValidationStatus) String() string {
	switch s {
	case StatusValid:
		return "License is valid"
	case StatusInvalidFormat:
		return "Invalid license format"
	case StatusInvalidSignature:
		return "Signature verification failed"
	case StatusUIDMismatch:
		return "Cluster UID mismatch"
	case StatusExpired:
		return "License expired"
	case StatusTampered:
		return "Hash chain integrity compromised"
	case StatusEnvError:
		return "Environment error"
	case StatusInternalError:
		return "Internal process error"
	default:
		return "Unknown status"
	}
}

// 预定义错误，方便判断和返回
var (
	ErrLicenseTampered       = fmt.Errorf("license hash chain tampered")
	ErrLicenseExpired        = fmt.Errorf("license expired")
	ErrLicenseUIDNotMatch    = fmt.Errorf("license UID mismatch")
	ErrLicenseSignatureWrong = fmt.Errorf("license signature verification failed")
	ErrInvalidLogEntry       = fmt.Errorf("invalid log entry")
)
