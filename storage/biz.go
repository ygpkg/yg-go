package storage

// InitMultipartUploadRequest 分片上传请求
type InitMultipartUploadRequest struct {
	SHA1     string `json:"sha1"`
	Size     int64  `json:"size"`
	Filename string `json:"filename"`

	// 仅后台使用
	CompanyID  uint   `json:"-"`
	CustomerID uint   `json:"-"`
	EmployeeID uint   `json:"-"`
	UploadID   string `json:"-"`
}

// CheckMultipartUploadResponse 检查分片上传响应
type CheckMultipartUploadResponse struct {
	// UploadID 上传ID, 用于后续上传分片
	UploadID string `json:"upload_id"`
	// PartSize 分片大小
	PartSize int64 `json:"part_size"`
	// PartCount 分片数量
	PartCount int `json:"part_count"`
	// PartNumbersUploaded 已上传的分片
	PartNumbersUploaded []int `json:"uploaded"`
	// PartNumbersNeedUpload 需要上传的分片
	PartNumbersNeedUpload []int `json:"need_upload"`
	// UploadStatus 上传状态
	UploadStatus FileStatus `json:"upload_status"`
	// FileInfo 文件信息
	FileInfo *FileInfo `json:"file_info"`
}
