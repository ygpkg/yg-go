package worker_test

import (
	"context"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// resultHandler 实现 ResultProvider：本次执行的结果快照由 handler 自己给出。
type resultHandler struct {
	payload []byte
	err     error
}

func (h *resultHandler) Execute(context.Context, *asynctask.AsyncTask) error { return h.err }

func (h *resultHandler) TakeResult() []byte { return h.payload }

// plainHandler 不实现 ResultProvider：行为必须与打补丁前一致（result 为空）。
type plainHandler struct{}

func (plainHandler) Execute(context.Context, *asynctask.AsyncTask) error { return nil }

// TestResultProviderPersisted 钉住「handler 能交回结果快照」这条能力。
//
// 为什么重要：下游（如 mutistitch 的 job.Get）要把执行结果（{schema,data,outputs}）
// 落进 core_async_task.result；没有这条通道，任务中心的 result 永远是空的。
func TestResultProviderPersisted(t *testing.T) {
	src := newMemSource()
	src.push("t1", &asynctask.AsyncTask{ID: "task-1", Type: "t1", MaxAttempts: 1})
	want := []byte(`{"schema":"v1","data":{"ok":true}}`)
	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:  1,
		LeaseTTL:     time.Second,
		PollInterval: 10 * time.Millisecond,
		WorkerID:     "w-1",
	})
	if err := w.Register("t1", &resultHandler{payload: want}); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = w.Shutdown(context.Background()) }()

	deadline := time.After(2 * time.Second)
	for {
		src.mu.Lock()
		results := append([]recordedResult(nil), src.results...)
		src.mu.Unlock()
		if len(results) > 0 {
			got := results[0]
			if got.Outcome != asynctask.OutcomeSucceeded {
				t.Fatalf("outcome = %s，want succeeded", got.Outcome)
			}
			if string(got.Payload.Payload) != string(want) {
				t.Fatalf("结果快照 = %s，want %s", got.Payload.Payload, want)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("2s 内没有产生 Save（worker 没跑到）")
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestResultEmptyWithoutProvider 守住向后兼容：不实现 ResultProvider 的 handler 行为不变。
func TestResultEmptyWithoutProvider(t *testing.T) {
	src := newMemSource()
	src.push("t2", &asynctask.AsyncTask{ID: "task-2", Type: "t2", MaxAttempts: 1})
	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:  1,
		LeaseTTL:     time.Second,
		PollInterval: 10 * time.Millisecond,
		WorkerID:     "w-2",
	})
	if err := w.Register("t2", plainHandler{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = w.Shutdown(context.Background()) }()

	deadline := time.After(2 * time.Second)
	for {
		src.mu.Lock()
		results := append([]recordedResult(nil), src.results...)
		src.mu.Unlock()
		if len(results) > 0 {
			if len(results[0].Payload.Payload) != 0 {
				t.Fatalf("未实现 ResultProvider 时结果应为空，got %s", results[0].Payload.Payload)
			}
			return
		}
		select {
		case <-deadline:
			t.Fatal("2s 内没有产生 Save")
		case <-time.After(10 * time.Millisecond):
		}
	}
}
