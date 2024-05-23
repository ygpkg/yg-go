package apiobj

// BaseRequest base request
type BaseRequest struct {
	Cmd string `json:"cmd"`
	Env string `json:"env,omitempty"`
}

// BaseResponse base response
type BaseResponse struct {
	Code      uint32 `json:"code"`
	Message   string `json:"message,omitempty"`
	Env       string `json:"env,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

// QueryRequest query request
type QueryRequest struct {
	BaseRequest

	Request PageQuery
}

// DetailIdRequest detail base request
type DetailIdRequest struct {
	BaseRequest
	Request struct {
		ID uint `json:"id"`
	}
}

// DetailNameRequest detail base request
type DetailNameRequest struct {
	BaseRequest
	Request struct {
		ID   uint   `json:"id"`
		Name string `json:"name"`
	}
}
