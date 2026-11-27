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

func GetESLogger(cfg ESLoggerConfig) *ESLogger {
	return &ESLogger{
		name:          cfg.LoggerName,
		slowThreshold: cfg.SlowThreshold,
	}
}

type ESLoggerConfig struct {
	LoggerName    string        // 日志模块名称
	SlowThreshold time.Duration // 慢查询阈值
}

type ESLogger struct {
	name          string
	slowThreshold time.Duration
}

// logger 每次写入时解析分组, 使配置热重载后自动切到新的日志出口.
func (e *ESLogger) logger() *zap.SugaredLogger {
	return Get(e.name).Desugar().WithOptions(zap.AddCallerSkip(5)).Sugar()
}

func (e *ESLogger) LogRoundTrip(req *http.Request, res *http.Response, err error, start time.Time, dur time.Duration) error {
	ctx := req.Context()
	l := e.logger()

	reqID, ok := ctx.Value(string(contextKeyRequestID)).(string)
	if !ok {
		reqID = ""
	}

	// 获取查询的 HTTP method 和路径
	method := req.Method
	path := fmt.Sprintf("%s?%s", req.URL.Path, req.URL.RawQuery)
	ralCodeKey := "ralcode"
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
			zap.Int(ralCodeKey, realCode),
		)
		l.With(fields...).Error(dslBody)
		return err
	}

	fields = append(fields, zap.Int(ralCodeKey, realCode))
	var affectedRows int
	if res.Body != nil && res.Body != http.NoBody {
		bodyBytes, readErr := io.ReadAll(res.Body)
		defer func() {
			res.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}()
		if readErr != nil {
			fields = append(fields, zap.Error(readErr))
			l.With(fields...).Error(dslBody)
			return nil
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

	// Exists API（HEAD /_doc/{id}）：404 表示文档不存在，属正常结果，不当作错误日志
	if (method == http.MethodHead || method == http.MethodGet) && realCode == http.StatusNotFound {
		l.With(fields...).Debug(dslBody)
	} else if realCode != 200 && realCode != 201 {
		l.With(fields...).Error(dslBody)
	} else {
		l.With(fields...).Debug(dslBody)
	}

	if e.slowThreshold > 0 && dur > e.slowThreshold {
		l.With(fields...).Warnf("slow dsl: %s", dslBody)
	}
	return nil
}

func (e *ESLogger) RequestBodyEnabled() bool {
	return true
}

func (e *ESLogger) ResponseBodyEnabled() bool {
	return true
}
