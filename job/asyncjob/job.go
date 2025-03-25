package asyncjob

import (
	"time"

	"github.com/ygpkg/yg-go/encryptor"
	"github.com/ygpkg/yg-go/job"
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// CreateAsyncJob 新建异步任务
func CreateAsyncJob(db *gorm.DB, compid, uin uint, purpose string) (*job.AsyncJob, error) {
	ajob := &job.AsyncJob{
		JobUUID:   encryptor.GenerateUUID(),
		CompanyID: compid,
		Uin:       uin,
		Purpose:   purpose,
		JobStatus: job.JobStatusPending,
	}
	err := db.Create(ajob).Error
	if err != nil {
		logs.Errorf("[asyncjob] create job failed: %s]", err)
		return nil, err
	}
	return ajob, nil
}

// GetJobByUUID 获取任务
func GetJobByUUID(db *gorm.DB, uuid string) (*job.AsyncJob, error) {
	ejob := &job.AsyncJob{}
	err := db.Where("job_uuid = ?", uuid).Last(ejob).Error
	if err != nil {
		logs.Errorf("[asyncjob] get ejob failed: %s]", err)
		return nil, err
	}
	return ejob, nil
}

// UpdateJobStatus 更新任务状态
func UpdateJobStatus(db *gorm.DB, uuid string, output string, e error) (*job.AsyncJob, error) {
	ejob, err := GetJobByUUID(db, uuid)
	if err != nil {
		return nil, err
	}

	if e != nil {
		ejob.JobStatus = job.JobStatusFailed
		if ejob.ErrorMsg == "" {
			ejob.ErrorMsg = e.Error()
		} else {
			ejob.ErrorMsg += "; " + e.Error()
		}
	} else {
		if ejob.JobStatus == job.JobStatusPending {
			ejob.JobStatus = job.JobStatusSuccess
		}
	}
	if output != "" {
		ejob.Output = output
	}
	ejob.CostSeconds = int(time.Now().Sub(ejob.CreatedAt).Seconds())

	err = db.Save(ejob).Error
	if err != nil {
		logs.Errorf("[asyncjob] update job failed: %s]", err)
		return nil, err
	}
	return ejob, nil
}
