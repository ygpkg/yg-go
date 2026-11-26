package worker_test

// 本文件覆盖 worker 侧的「暂停」与「取消通知」语义：
//   - handler 用 worker.Pause 让出：outcome=paused、钩子 OnPause 在结算时被调用；
//   - 取消通知命中本地活跃执行表 → 取消本次执行；
//   - 晚到的旧取消（generation/attempt 不匹配）不影响新执行；
//   - 定时补查与登记后复查能在通知丢失/注册竞态时兜底。

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// pauseHandler 请求暂停，并实现 PauseLifecycle 记录结算钩子。
type pauseHandler struct {
	onPause  int32
	reason   atomic.Value
	retryFor time.Duration
}

func (h *pauseHandler) Execute(ctx context.Context, t *asynctask.AsyncTask) error {
	return worker.Pause(h.retryFor, "检查点已提交，让出算力")
}

func (h *pauseHandler) OnPause(ctx context.Context, t *asynctask.AsyncTask, reason string) error {
	atomic.AddInt32(&h.onPause, 1)
	h.reason.Store(reason)
	return nil
}

// TestWorker_PauseOutcome handler 返回 Pause → 交给 Source 的 Outcome 是 paused（不是 failed）。
func TestWorker_PauseOutcome(t *testing.T) {
	h := &pauseHandler{}
	src := newMemSource()
	src.push("tp", &asynctask.AsyncTask{ID: "jp", MaxAttempts: 3})

	w := worker.New(src, &worker.WorkerConfig{Concurrency: 1, PollInterval: 5 * time.Millisecond})
	if err := w.Register("tp", h); err != nil {
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

	res, _, hooks := src.snapshot()
	if res[0].Outcome != asynctask.OutcomePaused {
		t.Fatalf("outcome = %q, want paused（暂停不是失败）", res[0].Outcome)
	}
	if res[0].Payload.Failure != asynctask.FailurePaused {
		t.Fatalf("failure = %q, want paused", res[0].Payload.Failure)
	}
	if res[0].Payload.PauseReason != "检查点已提交，让出算力" {
		t.Fatalf("暂停原因 = %q", res[0].Payload.PauseReason)
	}
	if atomic.LoadInt32(&h.onPause) != 1 {
		t.Fatalf("OnPause 调用次数 = %d, want 1", h.onPause)
	}
	if len(hooks) != 1 || hooks[0] != "on_pause" {
		t.Fatalf("结算钩子 = %v, want [on_pause]", hooks)
	}
	if h.reason.Load() != "检查点已提交，让出算力" {
		t.Fatalf("OnPause 收到的原因 = %v", h.reason.Load())
	}
}

// TestWorker_PauseRetryAfter PauseError.RetryAfter 会换算成 PauseUntil（Source 写 next_run_at）。
func TestWorker_PauseRetryAfter(t *testing.T) {
	h := &pauseHandler{retryFor: time.Hour}
	src := newMemSource()
	src.push("tp", &asynctask.AsyncTask{ID: "jp2", MaxAttempts: 3})

	w := worker.New(src, &worker.WorkerConfig{Concurrency: 1, PollInterval: 5 * time.Millisecond})
	if err := w.Register("tp", h); err != nil {
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

	res, _, _ := src.snapshot()
	if res[0].Payload.PauseUntil.IsZero() {
		t.Fatal("RetryAfter>0 时应带 PauseUntil")
	}
}

// blockingHandler 一直阻塞到 ctx 被取消，用来观察「取消通知是否真的叫醒了执行」。
type blockingHandler struct {
	started  chan struct{}
	finished chan struct{}
	once     sync.Once
}

func newBlockingHandler() *blockingHandler {
	return &blockingHandler{started: make(chan struct{}), finished: make(chan struct{})}
}

func (h *blockingHandler) Execute(ctx context.Context, t *asynctask.AsyncTask) error {
	h.once.Do(func() { close(h.started) })
	select {
	case <-ctx.Done():
		close(h.finished)
		return ctx.Err()
	case <-time.After(3 * time.Second):
		close(h.finished)
		return nil
	}
}

// fakeWatcher 是可控的取消通知来源（模拟 PG LISTEN 的通道）。
type fakeWatcher struct {
	ch chan asynctask.CancelSignal
}

func newFakeWatcher() *fakeWatcher {
	return &fakeWatcher{ch: make(chan asynctask.CancelSignal, 8)}
}

func (w *fakeWatcher) Watch(ctx context.Context) (<-chan asynctask.CancelSignal, error) {
	return w.ch, nil
}

// statusSource 在 memSource 之上提供读取状态的扩展接口（取消补查用）。
type statusSource struct {
	*memSource
	mu     sync.Mutex
	status asynctask.Status
}

func (s *statusSource) setStatus(st asynctask.Status) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.status = st
}

func (s *statusSource) TaskStatus(ctx context.Context, id string) (asynctask.Status, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status, nil
}

// TestWorker_CancelSignal 取消通知命中在途执行：执行 ctx 被取消，结果记为 canceled。
func TestWorker_CancelSignal(t *testing.T) {
	h := newBlockingHandler()
	src := newMemSource()
	src.push("tc", &asynctask.AsyncTask{ID: "jc", MaxAttempts: 3})
	watcher := newFakeWatcher()

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:   1,
		PollInterval:  5 * time.Millisecond,
		CancelWatcher: watcher,
	})
	if err := w.Register("tc", h); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	select {
	case <-h.started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler 未开始执行")
	}
	// memSource 不推进 attempt，默认 0（0 在信号里表示通配）。
	watcher.ch <- asynctask.CancelSignal{TaskID: "jc"}
	select {
	case <-h.finished:
	case <-time.After(2 * time.Second):
		t.Fatal("取消通知未叫醒执行")
	}

	waitCond(t, func() bool {
		res, _, _ := src.snapshot()
		return len(res) >= 1
	}, 2*time.Second)
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	<-done

	res, _, _ := src.snapshot()
	if res[0].Outcome != asynctask.OutcomeCanceled {
		t.Fatalf("取消后 outcome = %q, want canceled", res[0].Outcome)
	}
}

// TestWorker_LateCancelIgnored 晚到的取消（generation 不匹配）不得影响当前执行（设计 §9 验证项 c）。
func TestWorker_LateCancelIgnored(t *testing.T) {
	h := newBlockingHandler()
	src := newMemSource()
	// 当前执行属于 generation=1。
	src.push("tc", &asynctask.AsyncTask{ID: "jc2", Generation: 1, MaxAttempts: 3})
	watcher := newFakeWatcher()

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:   1,
		PollInterval:  5 * time.Millisecond,
		CancelWatcher: watcher,
	})
	if err := w.Register("tc", h); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	select {
	case <-h.started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler 未开始执行")
	}
	// 旧 generation 的取消通知（用户取消后又重放/重投的场景）。
	watcher.ch <- asynctask.CancelSignal{TaskID: "jc2", Generation: 2, Attempt: 1}
	select {
	case <-h.finished:
		t.Fatal("晚到的旧取消不该中断当前执行")
	case <-time.After(200 * time.Millisecond):
	}

	// 当前 generation 的取消仍然生效。
	watcher.ch <- asynctask.CancelSignal{TaskID: "jc2", Generation: 1}
	select {
	case <-h.finished:
	case <-time.After(2 * time.Second):
		t.Fatal("匹配的取消通知未生效")
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	<-done
}

// TestWorker_CancelRecheck 监听丢通知时，定时补查发现任务已终态并取消执行。
func TestWorker_CancelRecheck(t *testing.T) {
	h := newBlockingHandler()
	base := newMemSource()
	base.push("tr", &asynctask.AsyncTask{ID: "jr", MaxAttempts: 3})
	src := &statusSource{memSource: base, status: asynctask.StatusRunning}

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:           1,
		PollInterval:          5 * time.Millisecond,
		CancelRecheckInterval: 20 * time.Millisecond,
	})
	if err := w.Register("tr", h); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	select {
	case <-h.started:
	case <-time.After(2 * time.Second):
		t.Fatal("handler 未开始执行")
	}
	// 任务被取消（通知没到），补查应当发现。
	src.setStatus(asynctask.StatusCanceled)
	select {
	case <-h.finished:
	case <-time.After(2 * time.Second):
		t.Fatal("定时补查未取消执行")
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	<-done
}

// TestWorker_CancelRecheckOnRegister 登记后立即复查：取消事务先提交、执行后登记的竞态被补上。
func TestWorker_CancelRecheckOnRegister(t *testing.T) {
	h := newBlockingHandler()
	base := newMemSource()
	base.push("tg", &asynctask.AsyncTask{ID: "jg", MaxAttempts: 3})
	// 执行侧一登记就查到「已取消」。
	src := &statusSource{memSource: base, status: asynctask.StatusCanceled}

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:           1,
		PollInterval:          5 * time.Millisecond,
		CancelRecheckInterval: time.Hour, // 只验证登记复查，不靠定时补查
	})
	if err := w.Register("tg", h); err != nil {
		t.Fatalf("register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	select {
	case <-h.finished:
	case <-time.After(2 * time.Second):
		t.Fatal("登记后复查未取消执行")
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	<-done
}
