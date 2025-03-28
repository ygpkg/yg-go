package exportjob

import (
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/encryptor"
	"github.com/ygpkg/yg-go/job"
	"github.com/ygpkg/yg-go/logs"
)

// CreateExportJob 新建导出任务
func CreateExportJob(compid, uin uint, purpose string, timeoutSeconds int) (*job.ExportJob, error) {
	job := &job.ExportJob{
		JobUUID:        encryptor.GenerateUUID(),
		CompanyID:      compid,
		Uin:            uin,
		Purpose:        purpose,
		ExportStatus:   job.JobStatusPending,
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
func GetJobByUUID(uuid string) (*job.ExportJob, error) {
	ejob := &job.ExportJob{}
	err := dbtools.Core().Where("job_uuid = ?", uuid).Last(ejob).Error
	if err != nil {
		logs.Errorf("[exportjob] get ejob failed: %s]", err)
		return nil, err
	}
	return ejob, nil
}
