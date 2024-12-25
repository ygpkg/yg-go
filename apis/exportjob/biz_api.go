package exportjob

import (
	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/errcode"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/storage"
)

// DetailExportJob 获取导出任务
func DetailExportJob(ctx *gin.Context, req *DetailExportJobRequest, resp *DetailExportJobResponse) {
	job, err := GetJobByUUID(req.Request.JobID)
	if err != nil {
		logs.Errorf("[exportjob] get job by uuid failed, %v", err)
		resp.Message = err.Error()
		resp.Code = errcode.ErrCode_InternalError
		return
	}

	if job.UserID != runtime.AccountID(ctx) {
		resp.Message = "无权访问"
		resp.Code = errcode.ErrCode_NoPermission
		return
	} else if job.Output == "" {
		resp.Message = "导出任务为空"
		resp.Code = errcode.ErrCode_BadRequest
		return
	}

	resp.Response.JobID = job.JobUUID
	resp.Response.Status = job.ExportStatus
	resp.Response.ErrMsg = job.ErrorMsg

	s, err := storage.LoadStorager(job.Purpose)
	if err != nil {
		logs.Errorf("[exportjob] load storage failed, %v", err)
		resp.Message = err.Error()
		resp.Code = errcode.ErrCode_InternalError
		return
	}
	fileURL, err := s.GetPresignedURL(job.Output)
	if err != nil {
		logs.Errorf("[exportjob] get presigned url failed, %v", err)
		resp.Message = err.Error()
		resp.Code = errcode.ErrCode_InternalError
		return
	}
	resp.Response.FileURL = fileURL
}
