package job

import (
	"github.com/ygpkg/yg-go/config"
	"gorm.io/gorm"
)

// ExportJob 导出任务
type ExportJob struct {
	gorm.Model
	// JobUUID
	JobUUID   string `gorm:"column:job_uuid;type:varchar(36);not null;index"`
	CompanyID uint   `gorm:"column:company_id;type:int;not null;index"`
	Uin       uint   `gorm:"column:uin;type:int;not null;index"`
	// Purpose 导出类型,按业务分类，需要和导出文件上传 storage.Storager 的类型一致
	Purpose config.FilePurpose `gorm:"column:purpose;type:varchar(255);not null;index"`
	// ExportStatus 导出状态
	ExportStatus JobStatus `gorm:"column:export_status;type:varchar(20);not null;index"`
	// CostSeconds 耗时
	CostSeconds int `gorm:"column:cost_seconds;type:int;not null"`
	// TimeoutSeconds 超时时间
	TimeoutSeconds int `gorm:"column:timeout_seconds;type:int;not null"`
	// Output 输出路径, 使用 storage.FileInfo.StoragePath 获取
	Output string `gorm:"column:output;type:varchar(255);not null"`
	// ErrorMsg 错误信息
	ErrorMsg string `gorm:"column:error_msg;type:varchar(255)"`
}

// TableName 表名
func (ExportJob) TableName() string {
	return "core_export_jobs"
}
