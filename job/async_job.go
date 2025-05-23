package job

import (
	"gorm.io/gorm"
)

// AsyncJob 异步任务
type AsyncJob struct {
	gorm.Model
	// JobUUID
	JobUUID string `gorm:"column:job_uuid;type:varchar(36);not null;index"`
	// CompanyID uint   `gorm:"column:company_id;type:int;not null;index"`
	Uin uint `gorm:"column:uin;type:int;not null;index"`
	// Purpose 任务类型
	Purpose string `gorm:"column:purpose;type:varchar(255);not null;index"`
	// BusinessID 业务数据ID
	BusinessID uint `gorm:"column:business_id;type:bigint;not null;index"`
	// JobStatus 导出状态
	JobStatus JobStatus `gorm:"column:job_status;type:varchar(20);not null;index"`
	// ErrorMsg 错误信息
	ErrorMsg string `gorm:"column:error_msg;type:varchar(255)"`
	// CostSeconds 耗时
	CostSeconds int `gorm:"column:cost_seconds;type:int;not null"`
	// 输入内容
	Input string `gorm:"column:input;type:varchar(255)"`
	// Output 输出内容
	Output string `gorm:"column:output;type:text"`
	// Extra 扩展字段
	Extra string `gorm:"column:extra;type:text"`
}

// TableName 表名
func (AsyncJob) TableName() string {
	return "core_async_jobs"
}
