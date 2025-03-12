package job

import (
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/types"
	"gorm.io/gorm"
)

// JobStatus 导出状态
type JobStatus = string

const (
	// JobStatusPending 等待导出
	JobStatusPending JobStatus = "pending"
	// JobStatusSuccess 成功
	JobStatusSuccess JobStatus = "success"
	// JobStatusFailed 失败
	JobStatusFailed JobStatus = "failed"
)

// Job 导出任务
type Job struct {
	gorm.Model
	// JobUUID 任务的唯一标识符
	JobUUID string `gorm:"column:job_uuid;type:varchar(36);not null;index"`
	// CompanyID 公司 ID
	CompanyID uint `gorm:"column:company_id;type:int;not null;index"`
	// Uin 用户 ID
	Uin uint `gorm:"column:uin;type:int;not null;index"`
	// Purpose 目的
	Purpose string `gorm:"column:purpose;type:varchar(255);not null;index"`
	// JobStatus 状态
	JobStatus JobStatus `gorm:"column:export_status;type:varchar(20);not null;index"`
	// CostSeconds 耗时
	CostSeconds int `gorm:"column:cost_seconds;type:int;not null"`
	// Output 结果
	Output string `gorm:"column:output;type:varchar(255);not null"`
	// ErrorMsg 错误信息
	ErrorMsg types.StringArray `gorm:"column:error_msg;type:varchar(1024)"`
	// Extra 扩展字段
	Extra string `gorm:"column:extra;type:text"`
}

// TableName 表名
func (j *Job) TableName() string {
	return "core_jobs"
}

func InitDB(db *gorm.DB) error {
	return dbtools.InitModel(db,
		&Job{},
	)
}
