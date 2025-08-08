package estool

type RequestError struct {
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

type ResponseBodyTaskItem struct {
	Index struct {
		Index  string       `json:"_index"`
		ID     string       `json:"_id"`
		Status int          `json:"status"`
		Error  RequestError `json:"error"`
	} `json:"index"`
	Update struct {
		Index  string       `json:"_index"`
		ID     string       `json:"_id"`
		Status int          `json:"status"`
		Error  RequestError `json:"error"`
	} `json:"update"`
}

type ResponseBody struct {
	HasError bool                    `json:"errors"`
	Task     int                     `json:"task"`
	Items    []*ResponseBodyTaskItem `json:"items"`
}

func (r ResponseBody) Errors() []string {
	errs := make([]string, 0)
	for _, item := range r.Items {
		if item.Update.Status > 299 {
			errs = append(errs, item.Update.Error.Type+": "+item.Update.Error.Reason)
		}
	}
	for _, item := range r.Items {
		if item.Index.Status > 299 {
			errs = append(errs, item.Index.Error.Type+": "+item.Index.Error.Reason)
		}
	}
	return errs
}
