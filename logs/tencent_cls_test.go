package logs

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	cls "github.com/tencentcloud/tencentcloud-cls-sdk-go"
	"github.com/ygpkg/yg-go/config"
)

type testLogWriter struct {
	closed atomic.Int32
	wait   chan struct{}
	err    error
}

func (*testLogWriter) Write(p []byte) (int, error) { return len(p), nil }
func (*testLogWriter) Sync() error                 { return nil }
func (w *testLogWriter) Close() error {
	w.closed.Add(1)
	if w.wait != nil {
		<-w.wait
	}
	return w.err
}

func TestApplyConfigOwnsWriters(t *testing.T) {
	t.Cleanup(func() { _ = Shutdown(context.Background()) })
	cls := &config.TencentCLSConfig{
		TencentConfig: config.TencentConfig{Endpoint: "example.com", SecretID: "id", SecretKey: "key"},
		TopicID:       "topic",
	}
	old := &testLogWriter{}
	if err := applyConfig("test", config.LogsConfig{"keep": {{Writer: "cls", TencentCLS: cls}}}, func(config.TencentCLSConfig) (managedSyncer, error) {
		return old, nil
	}, false); err != nil {
		t.Fatal(err)
	}
	previousKeep := Get("keep")
	created := &testLogWriter{}
	var attempts int
	cfg := config.LogsConfig{"keep": {{Writer: "tencent", TencentCLS: cls}}, "later": {{Writer: "tencentcls", TencentCLS: cls}}}
	err := applyConfig("test", cfg, func(config.TencentCLSConfig) (managedSyncer, error) {
		attempts++
		if attempts == 1 {
			return created, nil
		}
		return nil, errors.New("init failed")
	}, false)
	var entry *ConfigError
	if !errors.As(err, &entry) || entry.Group != "later" || entry.Index != 0 {
		t.Fatalf("expected identifiable failing entry, got %v", err)
	}
	if Get("keep") != previousKeep || old.closed.Load() != 0 || created.closed.Load() != 1 {
		t.Fatal("failed apply must preserve old config and close new writers")
	}
	if err := ApplyConfig("test", config.LogsConfig{"default": nil}); err != nil {
		t.Fatal(err)
	}
	if Get("keep") != previousKeep || old.closed.Load() != 0 {
		t.Fatal("unmodified group was replaced")
	}
	if err := ApplyConfig("test", config.LogsConfig{"keep": nil}); err != nil || old.closed.Load() != 1 {
		t.Fatalf("replaced writer should close exactly once: %v", err)
	}
}

func TestShutdownSharesResultAfterTimeout(t *testing.T) {
	want := errors.New("writer failed")
	writer := &testLogWriter{wait: make(chan struct{}), err: want}
	cls := &config.TencentCLSConfig{
		TencentConfig: config.TencentConfig{Endpoint: "example.com", SecretID: "id", SecretKey: "key"},
		TopicID:       "topic",
	}
	if err := applyConfig("test", config.LogsConfig{"default": {{Writer: "cls", TencentCLS: cls}}}, func(config.TencentCLSConfig) (managedSyncer, error) {
		return writer, nil
	}, false); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	err := Shutdown(ctx)
	close(writer.wait)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected deadline error, got %v", err)
	}
	for range 2 {
		if err := Shutdown(context.Background()); !errors.Is(err, want) {
			t.Fatalf("expected shared writer error, got %v", err)
		}
	}
	if writer.closed.Load() != 1 {
		t.Fatal("writer closed more than once")
	}
}

func TestReloadConfigRetainsPanicOnInvalidConfig(t *testing.T) {
	defer func() {
		if _, ok := recover().(*ConfigError); !ok {
			t.Fatal("expected legacy configuration panic")
		}
	}()
	_ = ReloadConfig("test", config.LogsConfig{"default": {{Writer: "cls"}}})
}

func TestTencentCLSShutdownWaitsForDelivery(t *testing.T) {
	delivered := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/structuredlog" || r.URL.Query().Get("topic_id") != "test-topic" {
			t.Errorf("unexpected CLS request URL: %s", r.URL)
		}
		select {
		case delivered <- struct{}{}:
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	cfg := config.LogsConfig{"default": {{Writer: "cls", TencentCLS: &config.TencentCLSConfig{
		TencentConfig: config.TencentConfig{Endpoint: endpoint, SecretID: "test-id", SecretKey: "test-key"},
		TopicID:       "test-topic",
	}}}}
	if err := ApplyConfig("test", cfg); err != nil {
		t.Fatal(err)
	}
	// Let the timer enter its linger sleep, otherwise a shutdown that lands
	// before the first sleep would finish fast even without the wake-up patch.
	time.Sleep(200 * time.Millisecond)
	Info("last line")
	// 1s is below the SDK linger (2s), so this only passes when Close wakes the
	// timer instead of sleeping it out.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Shutdown(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case <-delivered:
	default:
		t.Fatal("shutdown completed before the CLS endpoint received the log")
	}
}

func TestApplyConfigRejectsMalformedCLSEndpoint(t *testing.T) {
	previous := Get("default")
	cfg := config.LogsConfig{"default": {{Writer: "cls", TencentCLS: &config.TencentCLSConfig{
		TencentConfig: config.TencentConfig{Endpoint: "https://example.com", SecretID: "id", SecretKey: "key"},
		TopicID:       "topic",
	}}}}
	var entry *ConfigError
	if err := ApplyConfig("test", cfg); !errors.As(err, &entry) || !strings.Contains(entry.Error(), "invalid tencent_cls.endpoint") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
	if Get("default") != previous {
		t.Fatal("invalid endpoint replaced the existing logger")
	}
}

func TestTencentCLSDropsWhenFullAndReportsDeliveryFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	cfg := cls.GetDefaultAsyncProducerClientConfig()
	cfg.Endpoint = strings.TrimPrefix(server.URL, "http://")
	cfg.AccessKeyID, cfg.AccessKeySecret = "test-id", "test-key"
	// A 1-byte budget means the first enqueued batch already fills it. This also
	// pins the vendored accounting fix: if batch creation stops charging
	// producerLogGroupSize, the second write is accepted again.
	cfg.MaxBlockSec, cfg.TotalSizeLnBytes, cfg.LingerMs = 0, 1, 100
	producer, err := cls.NewAsyncProducerClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writer := &tencentClsWriteWyncer{client: producer, topicID: "test-topic", dropOnFull: true}
	// Keep the timer stopped until the writes finish so nothing drains in between.
	log := []byte(`{"msg":"test log"}`)
	n1, err1 := writer.Write(log)
	n2, err2 := writer.Write(log)
	rejected, pending := writer.rejected.Load(), writer.pending.Load()
	producer.Start()
	closeErr := writer.closeWithin(time.Second)
	if err1 != nil || err2 != nil || n1 != len(log) || n2 != len(log) || rejected != 1 || pending != 1 {
		t.Fatalf("expected one queued log and one drop, got %d queued, %d rejected, write errors %v and %v", pending, rejected, err1, err2)
	}
	if closeErr == nil || !strings.Contains(closeErr.Error(), "delivery failed for 1 logs") {
		t.Fatalf("expected delivery failure on close, got %v", closeErr)
	}
}

// TestCloseWithinReportsCloseErrorAndPendingTogether pins that a failed SDK
// close does not hide the logs that are still queued.
func TestCloseWithinReportsCloseErrorAndPendingTogether(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(300 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cfg := cls.GetDefaultAsyncProducerClientConfig()
	cfg.Endpoint = strings.TrimPrefix(server.URL, "http://")
	cfg.AccessKeyID, cfg.AccessKeySecret = "test-id", "test-key"
	// Tiny batches keep one send in flight and another queued, so the SDK close
	// check always observes work and the dequeue gap cannot return nil.
	cfg.LingerMs, cfg.MaxBatchSize = 100, 1
	producer, err := cls.NewAsyncProducerClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	producer.Start()
	writer := &tencentClsWriteWyncer{client: producer, topicID: "test-topic"}
	log := []byte(`{"msg":"slow log"}`)
	for i := 0; i < 3; i++ {
		if _, err := writer.Write(log); err != nil {
			t.Fatal(err)
		}
	}
	err = writer.closeWithin(50 * time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "close tencent CLS producer") {
		t.Fatalf("expected SDK close error, got %v", err)
	}
	if !strings.Contains(err.Error(), "still pending") {
		t.Fatalf("pending logs must be reported together with the close error, got %v", err)
	}
}
