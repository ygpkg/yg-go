package job

import (
	"time"

	"github.com/robfig/cron/v3"
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/types"
)

// 定时任务调度器
var stdCron *cron.Cron

// RegistryCronFunc 通用任务注册
func RegistryCronFunc(spec string, companyID, uin uint, purpose string, f func() (string, error)) {
	// 确保 stdCron 已初始化
	if stdCron == nil {
		stdCron = cron.New(cron.WithSeconds())
		stdCron.Start()
	}

	_, err := stdCron.AddFunc(spec, func() {
		defer func() {
			if r := recover(); r != nil {
				logs.Errorf("Crash during task execution: %v", r)
			}
		}()
		startTime := time.Now()

		// 生成 Job 任务
		job := Job{
			JobUUID:     types.GenerateID(),
			CompanyID:   companyID,
			Uin:         uin,
			Purpose:     purpose,
			JobStatus:   JobStatusPending,
			CostSeconds: 0,
			Output:      "",
			ErrorMsg:    "",
			Extra:       "{}",
		}

		// 存储 Job
		if err := dbtools.Core().Create(&job).Error; err != nil {
			logs.Errorf("Failed to store Job record: %v", err)
			return
		}

		// 执行任务
		output, taskErr := f()
		costTime := int(time.Since(startTime).Seconds())

		// 更新 Job 状态
		job.CostSeconds = costTime
		if taskErr != nil {
			job.JobStatus = JobStatusFailed
			job.ErrorMsg.Add(taskErr.Error())
		} else {
			job.JobStatus = JobStatusSuccess
			job.Output = output
		}

		// 更新 Job 记录
		if err := dbtools.Core().Save(&job).Error; err != nil {
			logs.Errorf("Failed to update Job record: %v", err)
		} else {
			logs.Info("[RegistryCronFunc] Task completed in %d seconds, status: %s", costTime, job.JobStatus)
		}
	})
	if err != nil {
		logs.Errorf("Failed to register a timed task: %v", err)
	}
}
