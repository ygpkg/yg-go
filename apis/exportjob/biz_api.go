package exportjob

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/errcode"
	"github.com/ygpkg/yg-go/apis/runtime"
	"github.com/ygpkg/yg-go/job"
	"github.com/ygpkg/yg-go/logs"
	"github.com/ygpkg/yg-go/storage"
)

// DetailExportJob 获取导出任务
func DetailExportJob(ctx *gin.Context, req *DetailExportJobRequest, resp *DetailExportJobResponse) {
	ejob, err := GetJobByUUID(req.Request.JobID)
	if err != nil {
		logs.Errorf("[exportjob] get job by uuid failed, %v", err)
		resp.Message = err.Error()
		resp.Code = errcode.ErrCode_InternalError
		return
	}

	if ejob.Uin != runtime.Uin(ctx) {
		resp.Message = "无权访问"
		resp.Code = errcode.ErrCode_NoPermission
		return
	}

	resp.Response.JobID = ejob.JobUUID
	resp.Response.Status = ejob.ExportStatus
	resp.Response.ErrMsg = ejob.ErrorMsg
	if ejob.Output == "" || ejob.ExportStatus != job.JobStatusSuccess {
		return
	}

	s, err := storage.LoadStorager(ejob.Purpose)
	if err != nil {
		logs.Errorf("[exportjob] load storage failed, %v", err)
		resp.Message = err.Error()
		resp.Code = errcode.ErrCode_InternalError
		return
	}
	fileURL, err := s.GetPresignedURL(http.MethodGet, ejob.Output)
	if err != nil {
		logs.Errorf("[exportjob] get presigned url failed, %v", err)
		resp.Message = err.Error()
		resp.Code = errcode.ErrCode_InternalError
		return
	}
	resp.Response.FileURL = fileURL
}
