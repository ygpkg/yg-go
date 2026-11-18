package worker_test

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// recordingHandler 实现 Handler + Lifecycle，记录各钩子被调用的次数与顺序。
type recordingHandler struct {
	execFn func(ctx context.Context, t *asynctask.AsyncTask) error

	mu        sync.Mutex
	hooks     []string
	execCalls int32
	skip      bool // BeforeExecute 返回 ErrSkip
}

func (h *recordingHandler) Execute(ctx context.Context, t *asynctask.AsyncTask) error {
	atomic.AddInt32(&h.execCalls, 1)
	h.record("Execute")
	if h.execFn != nil {
		return h.execFn(ctx, t)
	}
	return nil
}

func (h *recordingHandler) BeforeExecute(ctx context.Context, t *asynctask.AsyncTask) error {
	h.record("BeforeExecute")
	if h.skip {
		return worker.ErrSkip
	}
	return nil
}

func (h *recordingHandler) OnRetry(ctx context.Context, t *asynctask.AsyncTask, cause error) error {
	h.record("OnRetry")
	return nil
}

func (h *recordingHandler) OnSuccess(ctx context.Context, t *asynctask.AsyncTask) error {
	h.record("OnSuccess")
	return nil
}

func (h *recordingHandler) OnFailure(ctx context.Context, t *asynctask.AsyncTask, cause error) error {
	h.record("OnFailure")
	return nil
}

func (h *recordingHandler) OnCanceled(ctx context.Context, t *asynctask.AsyncTask) error {
	h.record("OnCanceled")
	return nil
}

func (h *recordingHandler) record(name string) {
	h.mu.Lock()
	h.hooks = append(h.hooks, name)
	h.mu.Unlock()
}

func (h *recordingHandler) hookCount(name string) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	n := 0
	for _, x := range h.hooks {
		if x == name {
			n++
		}
	}
	return n
}

// runWorker 构造并运行一个 worker，处理完毕后停机；返回 memSource 供断言。
func runWorker(t *testing.T, h *recordingHandler, taskType string, asyncTasks ...*asynctask.AsyncTask) *memSource {
	t.Helper()
	src := newMemSource()
	for _, j := range asyncTasks {
		src.push(taskType, j)
	}

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:  2,
		PollInterval: 5 * time.Millisecond,
	})
	if err := w.Register(taskType, h); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	// 轮询直到 memSource 记录到至少一次结果（或超时）。
	waitCond(t, func() bool {
		res, _, _ := src.snapshot()
		return len(res) >= 1
	}, 2*time.Second)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := w.Shutdown(stopCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return")
	}
	return src
}

func waitCond(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

// 1. 成功结局：Succeeded + OnSuccess。
func TestWorker_Succeeded(t *testing.T) {
	h := &recordingHandler{}
	src := runWorker(t, h, "invoice:issue", &asynctask.AsyncTask{ID: "j1", MaxAttempts: 3})

	res, _, _ := src.snapshot()
	if len(res) == 0 || res[0].Outcome != asynctask.OutcomeSucceeded {
		t.Fatalf("want succeeded, got %+v", res)
	}
	if h.hookCount("OnSuccess") != 1 {
		t.Fatalf("want OnSuccess once, got hooks=%v", h.hooks)
	}
	if atomic.LoadInt32(&h.execCalls) != 1 {
		t.Fatalf("want 1 exec, got %d", h.execCalls)
	}
}

// 2. 失败可重试：Failed + OnRetry（多次尝试）。
//
// 这里自己起 worker 而不用 runWorker：runWorker 一看到第一条结果就停机，
// 会与新语义（停机立即停止认领）竞争，导致「重试还没发生就断言」。
func TestWorker_FailedRetry(t *testing.T) {
	h := &recordingHandler{
		execFn: func(ctx context.Context, t *asynctask.AsyncTask) error {
			return errors.New("boom")
		},
	}
	src := newMemSource()
	src.push("invoice:issue", &asynctask.AsyncTask{ID: "j2", MaxAttempts: 3})

	w := worker.New(src, &worker.WorkerConfig{Concurrency: 1, PollInterval: 5 * time.Millisecond})
	if err := w.Register("invoice:issue", h); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	// 等到真的重试过（第二次 Execute 开始）。
	waitCond(t, func() bool { return atomic.LoadInt32(&h.execCalls) >= 2 }, 2*time.Second)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	<-done

	// 至少出现过一次 OnRetry 回调（第一次失败 attempt=1 < MaxAttempts=3）。
	if h.hookCount("OnRetry") == 0 {
		t.Fatalf("want at least one OnRetry, got hooks=%v", h.hooks)
	}
}

// 3. 次数用尽：最终 OnFailure。
func TestWorker_FailedExhausted(t *testing.T) {
	h := &recordingHandler{
		execFn: func(ctx context.Context, t *asynctask.AsyncTask) error {
			return errors.New("always fail")
		},
	}
	// MaxAttempts=2、retryUpperBound 小，确保很快耗尽并落到 OnFailure。
	src := newMemSource()
	src.retryUpperBound = 2
	src.push("t3", &asynctask.AsyncTask{ID: "j3", MaxAttempts: 2})

	w := worker.New(src, &worker.WorkerConfig{Concurrency: 1, PollInterval: 5 * time.Millisecond})
	if err := w.Register("t3", h); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	waitCond(t, func() bool { return h.hookCount("OnFailure") >= 1 }, 2*time.Second)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	<-done

	if h.hookCount("OnFailure") < 1 {
		t.Fatalf("want OnFailure, got hooks=%v", h.hooks)
	}
}

// 4. ErrSkip：skipped 结局，Execute 与 OnSuccess 都不被调用，callback 为空。
func TestWorker_Skip(t *testing.T) {
	h := &recordingHandler{skip: true}
	src := runWorker(t, h, "invoice:issue", &asynctask.AsyncTask{ID: "j4", MaxAttempts: 3})

	res, _, hooks := src.snapshot()
	if len(res) == 0 || res[0].Outcome != asynctask.OutcomeSkipped {
		t.Fatalf("want skipped, got %+v", res)
	}
	if res[0].Failure != asynctask.FailureSkipped {
		t.Fatalf("want FailureSkipped, got %v", res[0].Failure)
	}
	if atomic.LoadInt32(&h.execCalls) != 0 {
		t.Fatalf("Execute must not be called on skip, got %d", h.execCalls)
	}
	if h.hookCount("OnSuccess") != 0 {
		t.Fatalf("OnSuccess must not be called on skip, got hooks=%v", h.hooks)
	}
	// callback 为空 → memSource 不应记录任何钩子调用。
	if len(hooks) != 0 {
		t.Fatalf("want no callback on skip, got %v", hooks)
	}
}

// 5. panic：panic 结局，Failure=panic，error_msg 带栈顶。
func TestWorker_Panic(t *testing.T) {
	h := &recordingHandler{
		execFn: func(ctx context.Context, t *asynctask.AsyncTask) error {
			panic("kaboom")
		},
	}
	src := runWorker(t, h, "invoice:issue", &asynctask.AsyncTask{ID: "j5", MaxAttempts: 1})

	res, _, _ := src.snapshot()
	if len(res) == 0 {
		t.Fatal("no result")
	}
	found := false
	for _, r := range res {
		if r.Outcome == asynctask.OutcomeFailed && r.Failure == asynctask.FailurePanic {
			found = true
			if r.Err == nil {
				t.Fatal("panic result must carry err with stack")
			}
		}
	}
	if !found {
		t.Fatalf("want failed/panic, got %+v", res)
	}
}

// 生命周期违规用例。
func TestWorker_Lifecycle(t *testing.T) {
	t.Run("New nil src", func(t *testing.T) {
		w := worker.New(nil, nil)
		if err := w.Register("x", worker.HandlerFunc(func(context.Context, *asynctask.AsyncTask) error { return nil })); !errors.Is(err, asynctask.ErrInvalidConfig) {
			t.Fatalf("want ErrInvalidConfig, got %v", err)
		}
	})

	t.Run("duplicate type", func(t *testing.T) {
		w := worker.New(newMemSource(), nil)
		h := worker.HandlerFunc(func(context.Context, *asynctask.AsyncTask) error { return nil })
		if err := w.Register("a", h); err != nil {
			t.Fatal(err)
		}
		if err := w.Register("a", h); !errors.Is(err, asynctask.ErrDuplicateType) {
			t.Fatalf("want ErrDuplicateType, got %v", err)
		}
	})

	t.Run("no type run", func(t *testing.T) {
		w := worker.New(newMemSource(), nil)
		if err := w.Run(context.Background()); !errors.Is(err, asynctask.ErrNoType) {
			t.Fatalf("want ErrNoType, got %v", err)
		}
	})

	t.Run("register after run and double run", func(t *testing.T) {
		src := newMemSource()
		w := worker.New(src, &worker.WorkerConfig{PollInterval: 5 * time.Millisecond})
		if err := w.Register("a", worker.HandlerFunc(func(context.Context, *asynctask.AsyncTask) error { return nil })); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- w.Run(ctx) }()
		// 等 Run 真正开始。
		time.Sleep(20 * time.Millisecond)
		if err := w.Register("b", worker.HandlerFunc(func(context.Context, *asynctask.AsyncTask) error { return nil })); !errors.Is(err, asynctask.ErrWorkerAlreadyRunning) {
			t.Fatalf("want ErrWorkerAlreadyRunning on register after run, got %v", err)
		}
		if err := w.Run(context.Background()); !errors.Is(err, asynctask.ErrWorkerAlreadyRunning) {
			t.Fatalf("want ErrWorkerAlreadyRunning on double run, got %v", err)
		}
		cancel()
		<-done
	})

	t.Run("shutdown then run", func(t *testing.T) {
		w := worker.New(newMemSource(), nil)
		if err := w.Shutdown(context.Background()); err != nil {
			t.Fatalf("shutdown before run must be nil, got %v", err)
		}
		w2 := worker.New(newMemSource(), nil)
		_ = w2.Register("a", worker.HandlerFunc(func(context.Context, *asynctask.AsyncTask) error { return nil }))
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- w2.Run(ctx) }()
		time.Sleep(20 * time.Millisecond)
		_ = w2.Shutdown(context.Background())
		cancel()
		<-done
		if err := w2.Run(context.Background()); !errors.Is(err, asynctask.ErrWorkerStopped) {
			t.Fatalf("want ErrWorkerStopped, got %v", err)
		}
	})
}

// 停机放租约用例：在途任务超时未收尾时，Shutdown 对其调用 src.Renew。
func TestWorker_ShutdownReleasesLease(t *testing.T) {
	block := make(chan struct{})
	h := &recordingHandler{
		execFn: func(ctx context.Context, t *asynctask.AsyncTask) error {
			<-block // 卡住直到测试放行
			return nil
		},
	}
	src := newMemSource()
	src.push("t", &asynctask.AsyncTask{ID: "s1", Timeout: 10 * time.Second})

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:     1,
		PollInterval:    5 * time.Millisecond,
		ShutdownTimeout: 50 * time.Millisecond, // 短停机超时，触发放租约
	})
	if err := w.Register("t", h); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	// 等 Execute 真正开始。
	waitCond(t, func() bool { return atomic.LoadInt32(&h.execCalls) >= 1 }, 2*time.Second)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)

	if src.releaseCount() < 1 {
		t.Fatalf("want Release called for stuck task, got %d", src.releaseCount())
	}
	close(block)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return")
	}
}
