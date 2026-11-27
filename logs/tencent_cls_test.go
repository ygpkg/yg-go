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
	Info("last line")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
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
	cfg.MaxBlockSec, cfg.TotalSizeLnBytes, cfg.LingerMs = 0, 1, 100
	producer, err := cls.NewAsyncProducerClient(cfg)
	if err != nil {
		t.Fatal(err)
	}
	writer := &tencentClsWriteWyncer{client: producer, topicID: "test-topic", dropOnFull: true}
	// Keep the timer stopped until the writes finish so capacity cannot change between them.
	log := []byte(`{"msg":"test log"}`)
	n1, err1 := writer.Write(log)
	n2, err2 := writer.Write(log)
	// The SDK starts accounting for a batch when a second log is appended.
	n3, err3 := writer.Write(log)
	rejected, pending := writer.rejected.Load(), writer.pending.Load()
	producer.Start()
	closeErr := writer.closeWithin(time.Second)
	if err1 != nil || err2 != nil || err3 != nil || n1 != len(log) || n2 != len(log) || n3 != len(log) || rejected != 1 || pending != 2 {
		t.Fatalf("expected two queued logs and one drop, got %d queued, %d rejected, write errors %v, %v and %v", pending, rejected, err1, err2, err3)
	}
	if closeErr == nil || !strings.Contains(closeErr.Error(), "delivery failed for 2 logs") {
		t.Fatalf("expected delivery failure on close, got %v", closeErr)
	}
}
