package logs

import (
	"bytes"
	"io"
	"net/http"
	"strconv"
	"time"

	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

func NewEsLogger() *EsLog {
	return &EsLog{}
}

type EsLog struct {
	service string
}

func (e *EsLog) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	ctx := req.Context()
	end := start.Add(dur)

	// 获取查询的 HTTP method 和路径
	method := req.Method
	path := req.URL.Path
	realCode := res.StatusCode

	var fields []interface{}
	fields = append(fields,
		zap.String("service", e.service),
		zap.String("proto", "es"),
		zap.String("start_time", start.Format(time.RFC3339)),
		zap.String("end_time", end.Format(time.RFC3339)),
		zap.Int64("cost", dur.Milliseconds()),
		zap.String("dslMethod", method),
		zap.String("dslPath", path),
		zap.Int("real_code", realCode),
	)
	msg := "es execute success"
	if err != nil {
		realCode = -1
		msg = err.Error()
		fields = append(fields, zap.String("error", msg))
	}

	if req.Body != nil && req.Body != http.NoBody {
		var buf bytes.Buffer
		if req.GetBody != nil {
			b, _ := req.GetBody()
			buf.ReadFrom(b)
		} else {
			buf.ReadFrom(req.Body)
		}
		fields = append(fields, zap.String("dslBody", buf.String()))
	}
	var affectedRows int
	if res.Body != nil && res.Body != http.NoBody {
		bodyBytes, readErr := io.ReadAll(res.Body)
		defer func() {
			res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}()
		if readErr != nil {
			fields = append(fields, zap.String("error", readErr.Error()))
			ErrorContextw(ctx, "read es response body fail", fields)
			return readErr
		}

		var resBody map[string]any
		if err := jsoniter.Unmarshal(bodyBytes, &resBody); err == nil {
			if hits, ok := resBody["hits"].(map[string]any); ok {
				if total, ok := hits["total"].(map[string]any); ok {
					if value, ok := total["value"].(float64); ok {
						affectedRows = int(value)
					}
				}
			}
		}
		// 恢复 res.Body 给后续使用者
		fields = append(fields, zap.String("affected_rows", strconv.Itoa(affectedRows)))
	}
	if realCode != 200 {
		ErrorContextw(ctx, msg, fields...)
	} else {
		InfoContextw(ctx, msg, fields...)
	}
	return err
}

func (e *EsLog) RequestBodyEnabled() bool {
	return true
}

func (e *EsLog) ResponseBodyEnabled() bool {
	return true
}
