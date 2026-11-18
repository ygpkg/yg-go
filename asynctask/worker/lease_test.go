package worker_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// TestWorker_RenewsLeaseDuringLongExecution 执行期必须续租：
// 默认实现从不续租时，超过 LeaseTTL 的任务会被别的实例接管并双跑。
func TestWorker_RenewsLeaseDuringLongExecution(t *testing.T) {
	h := &recordingHandler{
		execFn: func(ctx context.Context, t *asynctask.AsyncTask) error {
			time.Sleep(120 * time.Millisecond) // 跨过多个续租周期
			return nil
		},
	}
	src := newMemSource()
	src.push("t", &asynctask.AsyncTask{ID: "renew-1", MaxAttempts: 3})

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:  1,
		PollInterval: 5 * time.Millisecond,
		LeaseTTL:     60 * time.Millisecond, // 续租周期 20ms
	})
	if err := w.Register("t", h); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	waitCond(t, func() bool { return atomic.LoadInt32(&h.execCalls) >= 1 }, 2*time.Second)
	waitCond(t, func() bool {
		res, _, _ := src.snapshot()
		return len(res) >= 1
	}, 2*time.Second)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	<-done

	if src.renewCount() < 2 {
		t.Fatalf("want >=2 Renew during 120ms execution (LeaseTTL=60ms), got %d", src.renewCount())
	}
	res, _, _ := src.snapshot()
	if len(res) == 0 || res[0].Outcome != asynctask.OutcomeSucceeded {
		t.Fatalf("want succeeded result, got %+v", res)
	}
}

// TestWorker_LeaseLostStopsExecution 续租失权：立即取消执行 ctx，且不再 Save（不跑钩子）。
func TestWorker_LeaseLostStopsExecution(t *testing.T) {
	h := &recordingHandler{
		execFn: func(ctx context.Context, t *asynctask.AsyncTask) error {
			// 正常实现应响应 ctx.Done()：失权时不跑完、不写结果。
			<-ctx.Done()
			return ctx.Err()
		},
	}
	src := newMemSource()
	src.renewErr = asynctask.ErrLeaseLost
	src.push("t", &asynctask.AsyncTask{ID: "lost-1", MaxAttempts: 3})

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:  1,
		PollInterval: 5 * time.Millisecond,
		LeaseTTL:     30 * time.Millisecond, // 续租周期 10ms
		EventHandler: nil,
	})
	if err := w.Register("t", h); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	waitCond(t, func() bool { return src.renewCount() >= 1 }, 2*time.Second)
	waitCond(t, func() bool { return atomic.LoadInt32(&h.execCalls) >= 1 }, 2*time.Second)
	time.Sleep(50 * time.Millisecond) // 给 Save 一次被执行的机会

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	<-done

	if res, _, hooks := src.snapshot(); len(res) != 0 || len(hooks) != 0 {
		t.Fatalf("失权后不得落库/跑钩子，got results=%+v hooks=%v", res, hooks)
	}
}

// TestWorker_SingleFetcherPerType 每类型只有一个认领者：空闲轮询不随并发数放大。
func TestWorker_SingleFetcherPerType(t *testing.T) {
	src := newMemSource()
	const concurrency = 8
	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:  concurrency,
		PollInterval: 10 * time.Millisecond,
	})
	if err := w.Register("idle", worker.HandlerFunc(func(context.Context, *asynctask.AsyncTask) error { return nil })); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	time.Sleep(200 * time.Millisecond) // 约 20 个轮询周期

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	<-done

	// 旧实现：8 个槽各按 10ms 轮询 → 约 160 次。新实现：每类型 1 个 fetcher → 约 20 次。
	if n := src.fetches(); n > 60 {
		t.Fatalf("空闲期 Fetch 次数 = %d，疑似仍在「每槽独立轮询」（期望 ~20）", n)
	}
}

// TestWorker_ShutdownReleasesNotRenews 停机时放租约（Release），而不是续租（Renew）。
func TestWorker_ShutdownReleasesNotRenews(t *testing.T) {
	block := make(chan struct{})
	h := &recordingHandler{
		execFn: func(ctx context.Context, t *asynctask.AsyncTask) error {
			<-block
			return nil
		},
	}
	src := newMemSource()
	src.push("t", &asynctask.AsyncTask{ID: "release-1", MaxAttempts: 3, Timeout: 10 * time.Second})

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:     1,
		PollInterval:    5 * time.Millisecond,
		ShutdownTimeout: 30 * time.Millisecond,
		LeaseTTL:        time.Second,
	})
	if err := w.Register("t", h); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	waitCond(t, func() bool { return atomic.LoadInt32(&h.execCalls) >= 1 }, 2*time.Second)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)

	if src.releaseCount() < 1 {
		t.Fatalf("停机应对在途任务调用 Release，got %d", src.releaseCount())
	}
	close(block)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("run did not return")
	}
}

// TestWorker_ShutdownDoesNotBurnRetryAttempts 停机导致的取消不写终态，避免吞掉剩余重试次数。
func TestWorker_ShutdownDoesNotBurnRetryAttempts(t *testing.T) {
	entered := make(chan struct{})
	h := &recordingHandler{
		execFn: func(ctx context.Context, t *asynctask.AsyncTask) error {
			close(entered)
			<-ctx.Done()
			return ctx.Err()
		},
	}
	src := newMemSource()
	src.push("t", &asynctask.AsyncTask{ID: "burn-1", MaxAttempts: 3, Timeout: 10 * time.Second})

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:     1,
		PollInterval:    5 * time.Millisecond,
		ShutdownTimeout: 20 * time.Millisecond,
	})
	if err := w.Register("t", h); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not start")
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)

	waitCond(t, func() bool { return src.releaseCount() >= 1 }, 2*time.Second)
	cancel()
	<-done

	if res, _, _ := src.snapshot(); len(res) != 0 {
		t.Fatalf("停机取消不该写终态，got %+v", res)
	}
}

// TestWorker_FetchErrorDoesNotPanic Fetch 返回普通错误时按退避重试，不影响后续任务。
func TestWorker_FetchErrorRecovers(t *testing.T) {
	src := &flakySource{memSource: *newMemSource(), failLeft: 2}
	src.push("t", &asynctask.AsyncTask{ID: "flaky-1", MaxAttempts: 3})
	h := &recordingHandler{}

	w := worker.New(src, &worker.WorkerConfig{Concurrency: 1, PollInterval: 5 * time.Millisecond})
	if err := w.Register("t", h); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	waitCond(t, func() bool {
		res, _, _ := src.snapshot()
		return len(res) >= 1
	}, 2*time.Second)

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	<-done
}

// flakySource 前 failLeft 次 Fetch 返回普通错误，用于验证错误链路不影响后续认领。
type flakySource struct {
	memSource
	failLeft int
}

func (f *flakySource) Fetch(ctx context.Context, taskType string) (*asynctask.AsyncTask, error) {
	if f.failLeft > 0 {
		f.failLeft--
		return nil, errors.New("transient db error")
	}
	return f.memSource.Fetch(ctx, taskType)
}
