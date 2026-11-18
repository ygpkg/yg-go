package worker_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// concResultHandler 是并发安全的 ResultHandler：结果按任务 ID 计算，不依赖实例状态。
type concResultHandler struct{}

func (concResultHandler) ExecuteResult(_ context.Context, task *asynctask.AsyncTask) ([]byte, error) {
	return []byte(fmt.Sprintf(`{"id":%q}`, task.ID)), nil
}

// TestResultHandlerConcurrent 钉住「结果与本次调用绑定、并发安全」。
//
// 一个 Handler 实例被同类型的多个执行槽共享；若用无参的 TakeResult()，并发 >1 时
// 结果会串到别的任务上。ResultHandler 用返回值携带结果，天然不串。
func TestResultHandlerConcurrent(t *testing.T) {
	src := newMemSource()
	const n = 4
	for i := 0; i < n; i++ {
		src.push("t", &asynctask.AsyncTask{ID: fmt.Sprintf("task-%d", i), Type: "t", MaxAttempts: 1})
	}
	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:  n,
		LeaseTTL:     time.Second,
		PollInterval: 5 * time.Millisecond,
		WorkerID:     "w-conc",
	})
	if err := w.RegisterResult("t", concResultHandler{}); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() { _ = w.Shutdown(context.Background()) }()

	deadline := time.After(3 * time.Second)
	for {
		src.mu.Lock()
		results := append([]recordedResult(nil), src.results...)
		src.mu.Unlock()
		if len(results) >= n {
			for _, r := range results {
				want := fmt.Sprintf(`{"id":%q}`, r.AsyncTaskID)
				if string(r.Payload.Payload) != want {
					t.Fatalf("任务 %s 的结果 = %s，want %s（结果串到了别的任务）",
						r.AsyncTaskID, r.Payload.Payload, want)
				}
			}
			return
		}
		select {
		case <-deadline:
			t.Fatalf("3s 内只收到 %d/%d 条结果", len(results), n)
		case <-time.After(10 * time.Millisecond):
		}
	}
}
