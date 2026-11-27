package logs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"

	"github.com/ygpkg/yg-go/config"
)

// captureWriter records what reaches a log exit so tests can tell generations apart.
type captureWriter struct {
	mu     sync.Mutex
	lines  []string
	closed bool
}

func (w *captureWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return 0, errors.New("capture writer closed")
	}
	w.lines = append(w.lines, string(p))
	return len(p), nil
}

func (w *captureWriter) Sync() error { return nil }

func (w *captureWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.closed = true
	return nil
}

func (w *captureWriter) count() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.lines)
}

// TestLoggersObtainedBeforeReloadFollowNewExits checks that gorm/es adapters
// resolve the configured group on every write. They are created once and kept
// for the process lifetime, so a cached logger would keep writing to the exit
// closed by ApplyConfig.
func TestLoggersObtainedBeforeReloadFollowNewExits(t *testing.T) {
	t.Cleanup(func() { _ = Shutdown(context.Background()) })
	clsCfg := &config.TencentCLSConfig{
		TencentConfig: config.TencentConfig{Endpoint: "example.com", SecretID: "id", SecretKey: "key"},
		TopicID:       "topic",
	}
	cfg := config.LogsConfig{
		"es":   {{Writer: "cls", Level: zapcore.DebugLevel, TencentCLS: clsCfg}},
		"gorm": {{Writer: "cls", Level: zapcore.DebugLevel, TencentCLS: clsCfg}},
	}
	writers := make([]*captureWriter, 4)
	for i := range writers {
		writers[i] = &captureWriter{}
	}
	attempt := 0
	newCLS := func(config.TencentCLSConfig) (managedSyncer, error) {
		w := writers[attempt]
		attempt++
		return w, nil
	}
	// Groups are built in sorted order: es then gorm.
	if err := applyConfig("test", cfg, newCLS, false); err != nil {
		t.Fatal(err)
	}
	firstES, firstGorm := writers[0], writers[1]
	secondES, secondGorm := writers[2], writers[3]

	gl := GetGorm("gorm")
	es := GetESLogger(ESLoggerConfig{LoggerName: "es"})
	trace(gl)
	roundTrip(t, es)
	if firstGorm.count() != 1 || firstES.count() != 1 {
		t.Fatalf("expected one line per exit, got gorm=%d es=%d", firstGorm.count(), firstES.count())
	}

	if err := applyConfig("test", cfg, newCLS, false); err != nil {
		t.Fatal(err)
	}
	// Same handles, new configuration: they must write to the new exits.
	trace(gl)
	roundTrip(t, es)
	if firstGorm.count() != 1 || firstES.count() != 1 {
		t.Fatalf("stale exits still used: gorm=%d es=%d", firstGorm.count(), firstES.count())
	}
	if secondGorm.count() != 1 || secondES.count() != 1 {
		t.Fatalf("expected one line on new exits, got gorm=%d es=%d", secondGorm.count(), secondES.count())
	}
}

func trace(gl interface {
	Trace(context.Context, time.Time, func() (string, int64), error)
}) {
	gl.Trace(context.Background(), time.Now(), func() (string, int64) { return "select 1", 1 }, nil)
}

func roundTrip(t *testing.T, es *ESLogger) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/idx/_search", strings.NewReader("{}"))
	res := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(`{"hits":{"total":{"value":1}}}`)),
	}
	if err := es.LogRoundTrip(req, res, nil, time.Now(), time.Millisecond); err != nil {
		t.Fatal(err)
	}
}
