package asyncjob

import (
	"time"

	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/encryptor"
	"github.com/ygpkg/yg-go/job"
	"github.com/ygpkg/yg-go/logs"
)

// CreateAsyncJob 新建异步任务
func CreateAsyncJob(compid, uin uint, purpose string) (*job.AsyncJob, error) {
	ajob := &job.AsyncJob{
		JobUUID:   encryptor.GenerateUUID(),
		CompanyID: compid,
		Uin:       uin,
		Purpose:   purpose,
		JobStatus: job.JobStatusPending,
	}
	err := dbtools.Core().Create(ajob).Error
	if err != nil {
		logs.Errorf("[asyncjob] create job failed: %s]", err)
		return nil, err
	}
	return ajob, nil
}

// GetJobByUUID 获取任务
func GetJobByUUID(uuid string) (*job.AsyncJob, error) {
	ejob := &job.AsyncJob{}
	err := dbtools.Core().Where("job_uuid = ?", uuid).Last(ejob).Error
	if err != nil {
		logs.Errorf("[asyncjob] get ejob failed: %s]", err)
		return nil, err
	}
	return ejob, nil
}

// UpdateJobStatus 更新任务状态
func UpdateJobStatus(uuid string, output string, e error) (*job.AsyncJob, error) {
	ejob, err := GetJobByUUID(uuid)
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

	err = dbtools.Core().Save(ejob).Error
	if err != nil {
		logs.Errorf("[asyncjob] update job failed: %s]", err)
		return nil, err
	}
	return ejob, nil
}
