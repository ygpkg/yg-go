package upload

import "github.com/ygpkg/yg-go/apis/apiobj"

type UploadImageResponse struct {
	apiobj.BaseResponse

	Response FileInfo
}

// FileInfo file info
type FileInfo struct {
	FileID uint `json:"file_id,omitempty"`
	// CustomerID 用户ID
	CustomerID uint `json:"customer_id,omitempty"`
	// URL 文件访问地址
	URL string `json:"url,omitempty"`
	// Filename 原始文件名
	Filename string `json:"filename,omitempty"`
}
