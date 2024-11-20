package exportjob

import "github.com/ygpkg/yg-go/apis/apiobj"

// DetailExportJobRequest 获取导出任务
type DetailExportJobRequest struct {
	apiobj.BaseRequest
	Request struct {
		JobID string `json:"job_id"`
	}
}

// DetailExportJobResponse 获取导出任务返回
type DetailExportJobResponse struct {
	apiobj.BaseResponse
	Response struct {
		JobID   string `json:"job_id"`
		FileURL string `json:"file_url"`
		Status  string `json:"status"`
		ErrMsg  string `json:"err_msg"`
	}
}
