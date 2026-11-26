package db

import (
	"encoding/json"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// LabelsToJSON 把 Labels map 序列化为落库 JSON 文本（空 map → ""，即 NULL/空）。
func LabelsToJSON(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	b, err := json.Marshal(labels)
	if err != nil {
		return ""
	}
	return string(b)
}

// asyncTaskRowToDomain 把数据库行 CoreAsyncTask 转为契约实体 asynctask.AsyncTask。
func asyncTaskRowToDomain(r *CoreAsyncTask) *asynctask.AsyncTask {
	if r == nil {
		return nil
	}
	uniq := ""
	if r.UniqKey != nil {
		uniq = *r.UniqKey
	}
	j := &asynctask.AsyncTask{
		ID:           r.ID,
		Type:         r.TaskType,
		Status:       asynctask.Status(r.Status),
		Priority:     int(r.Priority),
		UniqKey:      uniq,
		BizType:      r.BizType,
		BizID:        r.BizID,
		CompanyID:    r.CompanyID,
		SubjectKind:  r.SubjectKind,
		SubjectID:    r.SubjectID,
		ChainID:      r.ChainID,
		Step:         int(r.Step),
		PrevStep:     int(r.PrevStep),
		Generation:   int(r.Generation),
		Attempt:      int(r.Attempt),
		FailureCount: int(r.FailureCount),
		MaxAttempts:  int(r.MaxAttempts),
		Timeout:      time.Duration(r.TimeoutMs) * time.Millisecond,
		NextRunAt:    r.NextRunAt,
		LeaseUntil:   r.LeaseUntil,
		WorkerID:     r.WorkerID,
		StartedAt:    r.StartedAt,
		FinishedAt:   r.FinishedAt,
		Duration:     time.Duration(r.DurationMs) * time.Millisecond,
		LastError:    r.LastError,
		RequestID:    r.RequestID,
		TraceID:      r.TraceID,
		CreatedAt:    r.CreatedAt,
		UpdatedAt:    r.UpdatedAt,
	}
	if r.Payload != "" {
		j.Payload = []byte(r.Payload)
	}
	if r.Result != "" {
		j.Result = []byte(r.Result)
	}
	if r.Labels != "" {
		_ = json.Unmarshal([]byte(r.Labels), &j.Labels)
	}
	return j
}

// asyncTaskRowsToDomain 批量转换。
func asyncTaskRowsToDomain(rows []CoreAsyncTask) []*asynctask.AsyncTask {
	out := make([]*asynctask.AsyncTask, len(rows))
	for i := range rows {
		out[i] = asyncTaskRowToDomain(&rows[i])
	}
	return out
}

// runRowToDomain 把数据库行 CoreAsyncTaskRun 转为契约实体 asynctask.AsyncTaskRun。
func runRowToDomain(r *CoreAsyncTaskRun) *asynctask.AsyncTaskRun {
	if r == nil {
		return nil
	}
	jr := &asynctask.AsyncTaskRun{
		ID:          r.ID,
		AsyncTaskID: r.AsyncTaskID,
		Generation:  int(r.Generation),
		Attempt:     int(r.Attempt),
		Status:      asynctask.Status(r.Status),
		Failure:     asynctask.FailureKind(r.FailureKind),
		WorkerID:    r.WorkerID,
		StartedAt:   r.StartedAt,
		FinishedAt:  r.FinishedAt,
		Duration:    time.Duration(r.DurationMs) * time.Millisecond,
		Error:       r.ErrorMsg,
		RequestID:   r.RequestID,
		TraceID:     r.TraceID,
		CreatedAt:   r.CreatedAt,
	}
	if r.Result != "" {
		jr.Result = []byte(r.Result)
	}
	return jr
}

// runRowsToDomain 批量转换。
func runRowsToDomain(rows []CoreAsyncTaskRun) []*asynctask.AsyncTaskRun {
	out := make([]*asynctask.AsyncTaskRun, len(rows))
	for i := range rows {
		out[i] = runRowToDomain(&rows[i])
	}
	return out
}

// uniqKeyPtr 把 Go 侧 string 转为列值：空串 → nil（NULL，不参与唯一约束）。
func uniqKeyPtr(k string) *string {
	if k == "" {
		return nil
	}
	return &k
}

// UniqKeyPtr 把幂等键转为可空指针（空串 → nil），供 manager 提交侧复用。导出包装。
func UniqKeyPtr(k string) *string { return uniqKeyPtr(k) }

// AsyncTaskRowToDomain 把数据库行 CoreAsyncTask 转为契约实体 asynctask.AsyncTask。导出包装，供 manager 回查复用。
func AsyncTaskRowToDomain(r *CoreAsyncTask) *asynctask.AsyncTask { return asyncTaskRowToDomain(r) }
