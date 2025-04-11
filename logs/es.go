package logs

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
	"go.uber.org/zap"
)

func GetESLogger(lgrName string) *ESLogger {
	lw := Get(lgrName)
	return &ESLogger{
		l: lw.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar(),
	}
}

type ESLogger struct {
	l *zap.SugaredLogger
}

func (e *ESLogger) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	ctx := req.Context()

	reqID, ok := ctx.Value(string(contextKeyRequestID)).(string)
	if !ok {
		reqID = ""
	}

	// 获取查询的 HTTP method 和路径
	method := req.Method
	path := req.URL.Path
	realCode := res.StatusCode

	var fields []interface{}
	fields = append(fields,
		zap.String(string(contextKeyRequestID), reqID),
		zap.String("elapsed", fmt.Sprintf("%dms", dur.Nanoseconds()/1e6)),
		zap.String("dslMethod", method),
		zap.String("dslPath", path),
	)
	var dslBody string
	if req.Body != nil && req.Body != http.NoBody {
		var buf bytes.Buffer
		if req.GetBody != nil {
			b, _ := req.GetBody()
			buf.ReadFrom(b)
		} else {
			buf.ReadFrom(req.Body)
		}
		dslBody = buf.String()
	}

	if err != nil {
		realCode = -1
		fields = append(fields,
			zap.Error(err),
			zap.Int("realcode", realCode),
		)
		e.l.With(fields...).Error(dslBody)
		return err
	}

	fields = append(fields, zap.Int("realcode", realCode))
	var affectedRows int
	if res.Body != nil && res.Body != http.NoBody {
		bodyBytes, readErr := io.ReadAll(res.Body)
		defer func() {
			res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}()
		if readErr != nil {
			fields = append(fields, zap.Error(readErr))
			e.l.With(fields...).Error(dslBody)
			return err
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
	}
	fields = append(fields, zap.Int("rows", affectedRows))
	
	if realCode != 200 {
		e.l.With(fields...).Error(dslBody)
	} else {
		e.l.With(fields...).Info(dslBody)
	}
	return err
}

func (e *ESLogger) RequestBodyEnabled() bool {
	return true
}

func (e *ESLogger) ResponseBodyEnabled() bool {
	return true
}
