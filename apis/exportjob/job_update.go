package exportjob

import (
	"time"

	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/logs"
)

// UpdateJobStatus 更新任务状态
func UpdateJobStatus(uuid string, output string, e error) (*ExportJob, error) {
	job, err := GetJobByUUID(uuid)
	if err != nil {
		return nil, err
	}

	if job.TimeoutSeconds > 0 &&
		time.Since(job.CreatedAt).Seconds() > float64(job.TimeoutSeconds) {
		job.ExportStatus = ExportStatusFailed
		job.ErrorMsg = "timeout"
	}
	if e != nil {
		job.ExportStatus = ExportStatusFailed
		if job.ErrorMsg == "" {
			job.ErrorMsg = e.Error()
		} else {
			job.ErrorMsg += "; " + e.Error()
		}
	} else {
		if job.ExportStatus == ExportStatusPending {
			job.ExportStatus = ExportStatusSuccess
		}
	}
	if output != "" {
		job.Output = output
	}
	job.CostSeconds = int(job.UpdatedAt.Sub(job.CreatedAt).Seconds())

	err = dbtools.Core().Save(job).Error
	if err != nil {
		logs.Errorf("[exportjob] update job failed: %s]", err)
		return nil, err
	}
	return job, nil
}
