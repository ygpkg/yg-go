package asyncjob

import (
	"context"
	"time"

	"github.com/ygpkg/yg-go/encryptor"
	"github.com/ygpkg/yg-go/job"
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// CreateAsyncJob 新建异步任务
func CreateAsyncJob(ctx context.Context, db *gorm.DB, req *CreateJobRequest) (*job.AsyncJob, error) {
	if err := req.Validate(); err != nil {
		logs.ErrorContextf(ctx, "[asyncjob.CreateAsyncJob] validate job failed, err: %v, req:%s", err, logs.JSON(req))
		return nil, err
	}
	ajob := &job.AsyncJob{
		JobUUID:    encryptor.GenerateUUID(),
		Uin:        req.Uin,
		Purpose:    req.Purpose,
		BusinessID: req.BusinessID,
		JobStatus:  job.JobStatusPending,
		Input:      req.Input,
		Extra:      req.Extra,
	}
	if err := db.WithContext(ctx).Create(ajob).Error; err != nil {
		logs.ErrorContextf(ctx, "[asyncjob.CreateAsyncJob] create job failed, err: %v, req:%s", err, logs.JSON(req))
		return nil, err
	}
	return ajob, nil
}

// GetJobByUUID 获取任务
func GetJobByUUID(ctx context.Context, db *gorm.DB, jobUUID string) (*job.AsyncJob, error) {
	ejob := &job.AsyncJob{}
	err := db.WithContext(ctx).Where("job_uuid = ?", jobUUID).Last(ejob).Error
	if err != nil {
		logs.ErrorContextf(ctx, "[asyncjob.GetJobByUUID] get job failed, err: %v, jobUUID:%s", err, jobUUID)
		return nil, err
	}
	return ejob, nil
}

// BatchGetJobByUUIDs 批量获取任务
func BatchGetJobByUUIDs(ctx context.Context, db *gorm.DB, jobUUIDs []string) ([]job.AsyncJob, error) {
	var ejobs []job.AsyncJob
	err := db.WithContext(ctx).Where("job_uuid IN (?)", jobUUIDs).Find(&ejobs).Error
	if err != nil {
		logs.ErrorContextf(ctx, "[asyncjob.BatchGetJobByUUIDs] get jobs failed, err: %v, jobUUIDs:%s", err, logs.JSON(jobUUIDs))
		return nil, err
	}
	return ejobs, nil
}

// BatchGetJobStatusByUUIDs 批量获取任务状态
func BatchGetJobStatusByUUIDs(ctx context.Context, db *gorm.DB, jobUUIDs []string) (map[string]job.JobStatus, error) {
	var ejobs []job.AsyncJob
	err := db.WithContext(ctx).Select("job_uuid, job_status").Where("job_uuid IN (?)", jobUUIDs).Find(&ejobs).Error
	if err != nil {
		logs.ErrorContextf(ctx, "[asyncjob.BatchGetJobStatusByUUIDs] get jobs failed, err: %v, jobUUIDs:%s", err, logs.JSON(jobUUIDs))
		return nil, err
	}
	statusMap := make(map[string]job.JobStatus)
	for _, ejob := range ejobs {
		statusMap[ejob.JobUUID] = ejob.JobStatus
	}
	return statusMap, nil
}

// UpdateJobStatus 更新任务状态
func UpdateJobStatus(ctx context.Context, db *gorm.DB, req *UpdateJobStatusRequest) (*job.AsyncJob, error) {
	if err := req.Validate(); err != nil {
		logs.ErrorContextf(ctx, "[asyncjob.UpdateJobStatus] validate job failed, err: %v, req:%s", err, logs.JSON(req))
		return nil, err
	}
	ejob, err := GetJobByUUID(ctx, db, req.JobUUID)
	if err != nil {
		return nil, err
	}

	updateMap := map[string]interface{}{
		"cost_seconds": int(time.Now().Sub(ejob.CreatedAt).Seconds()),
	}

	if req.Error != nil {
		updateMap["job_status"] = job.JobStatusFailed
		if ejob.ErrorMsg == "" {
			updateMap["error_msg"] = req.Error.Error()
		} else {
			updateMap["error_msg"] = ejob.ErrorMsg + "; " + req.Error.Error()
		}
	} else {
		updateMap["job_status"] = job.JobStatusSuccess
	}
	if req.Output != "" {
		updateMap["output"] = req.Output
	}

	err = db.WithContext(ctx).Model(&job.AsyncJob{}).Where("id = ?", ejob.ID).Updates(updateMap).Error
	if err != nil {
		logs.ErrorContextf(ctx, "[asyncjob.UpdateJobStatus] update job failed, err: %v, req:%s", err, logs.JSON(req))
		return nil, err
	}
	return ejob, nil
}
