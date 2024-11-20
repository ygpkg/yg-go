package exportjob

import (
	"time"

	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/lifecycle"
	"github.com/ygpkg/yg-go/logs"
)

// MonitorExportJobRoutine 监控导出任务
func MonitorExportJobRoutine() {
	defer func() {
		if err := recover(); err != nil {
			logs.Errorf("[exportjob] monitor routine panic: %s", err)
		}
	}()
	if err := dbtools.Core().AutoMigrate(&ExportJob{}); err != nil {
		logs.Errorf("[exportjob] monitor routine auto migrate failed: %s", err)
		return
	}
	for {
		select {
		case <-lifecycle.Std().C():
			logs.Infof("[exportjob] monitor routine exit")
			return
		default:
			repairTimeoutJobs()
		}
	}
}

// repairTimeoutJobs 修复超时任务
func repairTimeoutJobs() {
	jobs := []*ExportJob{}
	err := dbtools.Core().Where("export_status = ? AND timeout_seconds > 0", ExportStatusPending).
		Find(&jobs).Error
	if err != nil {
		logs.Errorf("[exportjob] repair timeout jobs failed: %s", err)
		return
	}
	for _, job := range jobs {
		timeout, err := checkTimeoutJob(job)
		if err != nil {
			logs.Errorf("[exportjob] repair timeout job failed: %s", err)
			return
		}
		if timeout {
			logs.Infof("[exportjob] repair timeout job: %s", job.JobUUID)
		}
	}
}

func checkTimeoutJob(job *ExportJob) (bool, error) {
	timeout := false
	if job.TimeoutSeconds > 0 &&
		time.Since(job.CreatedAt).Seconds() > float64(job.TimeoutSeconds) {
		timeout = true
		job.ExportStatus = ExportStatusFailed
		job.ErrorMsg = "timeout"
		return timeout, dbtools.Core().Save(job).Error
	}

	return timeout, nil
}
