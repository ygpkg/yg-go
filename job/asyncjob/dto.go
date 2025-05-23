package asyncjob

import "errors"

type CreateJobRequest struct {
	Uin        uint   `json:"uin"`         // 用户uin
	Purpose    string `json:"purpose"`     // 任务类型
	BusinessID uint   `json:"business_id"` // 业务数据ID
	Input      string `json:"input"`       // 输入内容
	Extra      string `json:"extra"`       // 扩展信息
}

func (req *CreateJobRequest) Validate() error {
	if req == nil {
		return errors.New("request is nil")
	}
	if req.Purpose == "" {
		return errors.New("purpose is empty")
	}
	return nil
}

type UpdateJobStatusRequest struct {
	JobUUID string `json:"job_uuid"` // 任务UUID
	Error   error  `json:"error"`    // 错误信息
	Output  string `json:"output"`   // 输出内容
	Extra   string `json:"extra"`    // 扩展信息
}

func (req *UpdateJobStatusRequest) Validate() error {
	if req == nil {
		return errors.New("request is nil")
	}
	if req.JobUUID == "" {
		return errors.New("job_uuid is empty")
	}
	return nil
}
