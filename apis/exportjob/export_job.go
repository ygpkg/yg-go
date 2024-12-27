package exportjob

import (
	"github.com/ygpkg/yg-go/config"
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/encryptor"
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// ExportStatus 导出状态
type ExportStatus = string

const (
	// ExportStatusPending 等待导出
	ExportStatusPending ExportStatus = "pending"
	// ExportStatusSuccess 成功
	ExportStatusSuccess ExportStatus = "success"
	// ExportStatusFailed 失败
	ExportStatusFailed ExportStatus = "failed"
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
	ExportStatus ExportStatus `gorm:"column:export_status;type:varchar(20);not null;index"`
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

// CreateExportJob 新建导出任务
func CreateExportJob(compid, uin uint, purpose string, timeoutSeconds int) (*ExportJob, error) {
	job := &ExportJob{
		JobUUID:        encryptor.GenerateUUID(),
		CompanyID:      compid,
		Uin:            uin,
		Purpose:        purpose,
		ExportStatus:   ExportStatusPending,
		TimeoutSeconds: timeoutSeconds,
	}
	err := dbtools.Core().Create(job).Error
	if err != nil {
		logs.Errorf("[exportjob] create job failed: %s]", err)
		return nil, err
	}
	return job, nil
}

// GetJobByUUID 获取任务
func GetJobByUUID(uuid string) (*ExportJob, error) {
	job := &ExportJob{}
	err := dbtools.Core().Where("job_uuid = ?", uuid).Last(job).Error
	if err != nil {
		logs.Errorf("[exportjob] get job failed: %s]", err)
		return nil, err
	}
	return job, nil
}
