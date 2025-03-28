package exportjob

import (
	"time"

	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/job"
	"github.com/ygpkg/yg-go/logs"
)

// UpdateJobStatus 更新任务状态
func UpdateJobStatus(uuid string, output string, e error) (*job.ExportJob, error) {
	ejob, err := GetJobByUUID(uuid)
	if err != nil {
		return nil, err
	}

	if ejob.TimeoutSeconds > 0 &&
		time.Since(ejob.CreatedAt).Seconds() > float64(ejob.TimeoutSeconds) {
		ejob.ExportStatus = job.JobStatusFailed
		ejob.ErrorMsg = "timeout"
	}
	if e != nil {
		ejob.ExportStatus = job.JobStatusFailed
		if ejob.ErrorMsg == "" {
			ejob.ErrorMsg = e.Error()
		} else {
			ejob.ErrorMsg += "; " + e.Error()
		}
	} else {
		if ejob.ExportStatus == job.JobStatusPending {
			ejob.ExportStatus = job.JobStatusSuccess
		}
	}
	if output != "" {
		ejob.Output = output
	}

	ejob.CostSeconds = int(time.Now().Sub(ejob.CreatedAt).Seconds())

	err = dbtools.Core().Save(ejob).Error
	if err != nil {
		logs.Errorf("[exportjob] update job failed: %s]", err)
		return nil, err
	}
	return ejob, nil
}
