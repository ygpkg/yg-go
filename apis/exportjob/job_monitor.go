package exportjob

import (
	"time"

	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/job"
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
	if err := dbtools.Core().AutoMigrate(&job.ExportJob{}); err != nil {
		logs.Errorf("[exportjob] monitor routine auto migrate failed: %s", err)
		return
	}
	interval := time.Second * 10
	tmr := time.NewTimer(interval)
	for {
		select {
		case <-lifecycle.Std().C():
			logs.Infof("[exportjob] monitor routine exit")
			return
		case <-tmr.C:
			repairTimeoutJobs()
			tmr.Reset(interval)
		}
	}
}

// repairTimeoutJobs 修复超时任务
func repairTimeoutJobs() {
	jobs := []*job.ExportJob{}
	err := dbtools.Core().Where("export_status = ? AND timeout_seconds > 0", job.JobStatusPending).
		Find(&jobs).Error
	if err != nil {
		logs.Errorf("[exportjob] repair timeout jobs failed: %s", err)
		return
	}
	for _, j := range jobs {
		timeout, err := checkTimeoutJob(j)
		if err != nil {
			logs.Errorf("[exportjob] repair timeout job failed: %s", err)
			return
		}
		if timeout {
			logs.Infof("[exportjob] repair timeout job: %s", j.JobUUID)
		}
	}
}

func checkTimeoutJob(j *job.ExportJob) (bool, error) {
	timeout := false
	if j.TimeoutSeconds > 0 &&
		time.Since(j.CreatedAt).Seconds() > float64(j.TimeoutSeconds) {
		timeout = true
		j.ExportStatus = job.JobStatusFailed
		j.ErrorMsg = "timeout"
		return timeout, dbtools.Core().Save(j).Error
	}

	return timeout, nil
}
