package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// savedTaskResult SaveTaskResult 的调用记录
type savedTaskResult struct {
	taskID   uint
	result   string
	err      error
	callback func(context.Context) error
}

// mockWorkManager 可编程的 WorkManager，用于驱动 Worker 的各条路径
type mockWorkManager struct {
	mu       sync.Mutex
	queue    []TaskInfo
	getErr   error
	getCalls int32
	saved    []savedTaskResult
	savedCh  chan savedTaskResult
}

func newMockWorkManager() *mockWorkManager {
	return &mockWorkManager{
		savedCh: make(chan savedTaskResult, 16),
	}
}

func (m *mockWorkManager) push(tasks ...TaskInfo) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queue = append(m.queue, tasks...)
}

func (m *mockWorkManager) setGetErr(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.getErr = err
}

func (m *mockWorkManager) GetNextTask(ctx context.Context, taskType string, workerID string) (TaskInfo, error) {
	atomic.AddInt32(&m.getCalls, 1)

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.getErr != nil {
		return TaskInfo{}, m.getErr
	}
	if len(m.queue) == 0 {
		// 空任务：TaskID == 0，与 PG/HTTP 版 manager 的约定一致
		return TaskInfo{}, nil
	}
	info := m.queue[0]
	m.queue = m.queue[1:]
	return info, nil
}

func (m *mockWorkManager) SaveTaskResult(ctx context.Context, workerID string, info TaskInfo, result string, err error, onCallback func(context.Context) error) error {
	saved := savedTaskResult{
		taskID:   info.TaskID,
		result:   result,
		err:      err,
		callback: onCallback,
	}

	m.mu.Lock()
	m.saved = append(m.saved, saved)
	m.mu.Unlock()

	m.savedCh <- saved
	return nil
}

func (m *mockWorkManager) InitTaskDBStatus(ctx context.Context) error { return nil }

func (m *mockWorkManager) getCallCount() int32 {
	return atomic.LoadInt32(&m.getCalls)
}

func (m *mockWorkManager) waitSaved(t *testing.T, timeout time.Duration) savedTaskResult {
	t.Helper()
	select {
	case saved := <-m.savedCh:
		return saved
	case <-time.After(timeout):
		t.Fatalf("wait SaveTaskResult timeout after %v", timeout)
		return savedTaskResult{}
	}
}

// recordingExecutor 记录 Execute 收到的上下文信息
type recordingExecutor struct {
	mu           sync.Mutex
	ctxErr       error
	hasDeadline  bool
	executeCalls int32
	execute      func(ctx context.Context) error
}

func (e *recordingExecutor) Execute(ctx context.Context) error {
	atomic.AddInt32(&e.executeCalls, 1)

	e.mu.Lock()
	_, e.hasDeadline = ctx.Deadline()
	e.mu.Unlock()

	if e.execute != nil {
		err := e.execute(ctx)
		e.mu.Lock()
		e.ctxErr = err
		e.mu.Unlock()
		return err
	}
	return nil
}

func (e *recordingExecutor) GetResult() string                   { return "" }
func (e *recordingExecutor) SetResult(result string)             {}
func (e *recordingExecutor) OnSuccess(ctx context.Context) error { return nil }
func (e *recordingExecutor) OnFailure(ctx context.Context) error { return nil }

func (e *recordingExecutor) snapshot() (error, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.ctxErr, e.hasDeadline
}

// newTestWorkerConfig 返回一份可通过 Validate 的基础配置
func newTestWorkerConfig(idleInterval time.Duration) *WorkerConfig {
	return &WorkerConfig{
		Timeout:        10 * time.Minute,
		MaxRedo:        3,
		MaxConcurrency: 1,
		WorkerID:       "test-worker",
		IdleInterval:   idleInterval,
	}
}

// waitFor 轮询等待条件成立
func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

// TestWorker_IdleIntervalSleepCancelable 空闲退避可被 Stop 取消，且默认 0 保持旧行为
func TestWorker_IdleIntervalSleepCancelable(t *testing.T) {
	t.Run("idle interval sleep is cancellable", func(t *testing.T) {
		mgr := newMockWorkManager()
		w, err := NewWorker(newTestWorkerConfig(30*time.Second), mgr)
		if err != nil {
			t.Fatalf("NewWorker() error = %v", err)
		}
		w.RegisterExecutor("idle_task", func(payload string) (TaskExecutor, error) {
			return &recordingExecutor{}, nil
		})

		if err := w.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// 空队列：应当只取一次任务，然后进入 30s 的退避睡眠
		waitFor(t, 2*time.Second, func() bool { return mgr.getCallCount() >= 1 })
		time.Sleep(100 * time.Millisecond)
		if got := mgr.getCallCount(); got != 1 {
			t.Fatalf("expected 1 GetNextTask call while idling, got %d (hot loop?)", got)
		}

		// Stop 必须能打断退避睡眠（若不可取消，这里会等满 30s）
		start := time.Now()
		if err := w.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
		if elapsed := time.Since(start); elapsed > 2*time.Second {
			t.Fatalf("Stop() took %v, idle sleep was not cancellable", elapsed)
		}
		if got := mgr.getCallCount(); got != 1 {
			t.Fatalf("expected 1 GetNextTask call after Stop, got %d", got)
		}
	})

	t.Run("zero idle interval keeps legacy hot loop", func(t *testing.T) {
		mgr := newMockWorkManager()
		w, err := NewWorker(newTestWorkerConfig(0), mgr)
		if err != nil {
			t.Fatalf("NewWorker() error = %v", err)
		}
		w.RegisterExecutor("idle_task", func(payload string) (TaskExecutor, error) {
			return &recordingExecutor{}, nil
		})

		if err := w.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v", err)
		}

		// IdleInterval=0 与旧版本等价：空任务立即重试
		waitFor(t, 2*time.Second, func() bool { return mgr.getCallCount() >= 20 })

		if err := w.Stop(context.Background()); err != nil {
			t.Fatalf("Stop() error = %v", err)
		}
	})
}

// TestWorker_TaskInfoTimeout 任务自带 Timeout 时，执行被 context 超时约束
func TestWorker_TaskInfoTimeout(t *testing.T) {
	mgr := newMockWorkManager()
	// 退避间隔取小值，避免任务执行完后空队列空转
	w, err := NewWorker(newTestWorkerConfig(20*time.Millisecond), mgr)
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}

	executor := &recordingExecutor{
		execute: func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
	}
	w.RegisterExecutor("timeout_task", func(payload string) (TaskExecutor, error) {
		return executor, nil
	})

	mgr.push(TaskInfo{
		TaskID:   42,
		TaskType: "timeout_task",
		Payload:  "p",
		Timeout:  50 * time.Millisecond,
	})

	start := time.Now()
	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer w.Stop(context.Background())

	saved := mgr.waitSaved(t, 5*time.Second)
	if elapsed := time.Since(start); elapsed > 3*time.Second {
		t.Fatalf("task should be interrupted by its own Timeout, took %v", elapsed)
	}

	if !errors.Is(saved.err, context.DeadlineExceeded) {
		t.Fatalf("expected SaveTaskResult err = context.DeadlineExceeded, got %v", saved.err)
	}
	if saved.callback == nil {
		t.Fatal("expected OnFailure callback for timed out task")
	}
	ctxErr, hasDeadline := executor.snapshot()
	if !hasDeadline {
		t.Fatal("expected Execute ctx to carry the task deadline")
	}
	if !errors.Is(ctxErr, context.DeadlineExceeded) {
		t.Fatalf("expected executor to observe context.DeadlineExceeded, got %v", ctxErr)
	}
}

// TestWorker_TaskInfoTimeoutZeroKeepsLegacy 默认 Timeout=0 时不加超时，行为与旧版本一致
func TestWorker_TaskInfoTimeoutZeroKeepsLegacy(t *testing.T) {
	mgr := newMockWorkManager()
	w, err := NewWorker(newTestWorkerConfig(20*time.Millisecond), mgr)
	if err != nil {
		t.Fatalf("NewWorker() error = %v", err)
	}

	executor := &recordingExecutor{}
	w.RegisterExecutor("legacy_task", func(payload string) (TaskExecutor, error) {
		return executor, nil
	})

	mgr.push(TaskInfo{TaskID: 7, TaskType: "legacy_task", Payload: "p"})

	if err := w.Start(context.Background()); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer w.Stop(context.Background())

	saved := mgr.waitSaved(t, 5*time.Second)
	if saved.err != nil {
		t.Fatalf("expected success for Timeout=0 task, got %v", saved.err)
	}
	if _, hasDeadline := executor.snapshot(); hasDeadline {
		t.Fatal("expected no deadline when TaskInfo.Timeout == 0")
	}
}

// TestWorker_ExecutorFactoryWithInfo Info 版工厂优先，且能拿到完整任务信息
func TestWorker_ExecutorFactoryWithInfo(t *testing.T) {
	t.Run("info factory takes precedence", func(t *testing.T) {
		mgr := newMockWorkManager()
		w, err := NewWorker(newTestWorkerConfig(20*time.Millisecond), mgr)
		if err != nil {
			t.Fatalf("NewWorker() error = %v", err)
		}

		var legacyCalled int32
		w.RegisterExecutor("info_task", func(payload string) (TaskExecutor, error) {
			atomic.AddInt32(&legacyCalled, 1)
			return &recordingExecutor{}, nil
		})

		infoExecutor := &recordingExecutor{}
		var gotInfo TaskInfo
		var infoCalls int32
		w.RegisterExecutorWithInfo("info_task", func(info TaskInfo) (TaskExecutor, error) {
			atomic.AddInt32(&infoCalls, 1)
			gotInfo = info
			return infoExecutor, nil
		}, WithConcurrency(2))

		mgr.push(TaskInfo{TaskID: 99, TaskType: "info_task", Payload: "payload", Timeout: 0})

		if err := w.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer w.Stop(context.Background())

		saved := mgr.waitSaved(t, 5*time.Second)
		if saved.taskID != 99 {
			t.Fatalf("expected saved task 99, got %d", saved.taskID)
		}
		if got := atomic.LoadInt32(&infoCalls); got != 1 {
			t.Fatalf("expected info factory called once, got %d", got)
		}
		if got := atomic.LoadInt32(&legacyCalled); got != 0 {
			t.Fatalf("expected legacy factory not called, got %d", got)
		}
		if gotInfo.TaskID != 99 || gotInfo.TaskType != "info_task" || gotInfo.Payload != "payload" {
			t.Fatalf("info factory got unexpected TaskInfo: %+v", gotInfo)
		}
		if !containsTaskType(w.registry.GetAll(), "info_task") {
			t.Fatal("expected info-registered task type to appear in GetAll")
		}
	})

	t.Run("legacy factory still works when no info factory", func(t *testing.T) {
		mgr := newMockWorkManager()
		w, err := NewWorker(newTestWorkerConfig(20*time.Millisecond), mgr)
		if err != nil {
			t.Fatalf("NewWorker() error = %v", err)
		}

		var payloadGot string
		w.RegisterExecutor("legacy_only", func(payload string) (TaskExecutor, error) {
			payloadGot = payload
			return &recordingExecutor{}, nil
		})

		mgr.push(TaskInfo{TaskID: 11, TaskType: "legacy_only", Payload: "legacy-payload"})

		if err := w.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer w.Stop(context.Background())

		saved := mgr.waitSaved(t, 5*time.Second)
		if saved.taskID != 11 {
			t.Fatalf("expected saved task 11, got %d", saved.taskID)
		}
		if payloadGot != "legacy-payload" {
			t.Fatalf("expected legacy factory payload, got %q", payloadGot)
		}
	})

	t.Run("factory error is saved as failure", func(t *testing.T) {
		mgr := newMockWorkManager()
		w, err := NewWorker(newTestWorkerConfig(20*time.Millisecond), mgr)
		if err != nil {
			t.Fatalf("NewWorker() error = %v", err)
		}

		w.RegisterExecutorWithInfo("broken", func(info TaskInfo) (TaskExecutor, error) {
			return nil, fmt.Errorf("boom")
		})

		mgr.push(TaskInfo{TaskID: 12, TaskType: "broken"})

		if err := w.Start(context.Background()); err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		defer w.Stop(context.Background())

		saved := mgr.waitSaved(t, 5*time.Second)
		if saved.err == nil {
			t.Fatal("expected factory error to be saved")
		}
	})
}

// TestWorkerConfig_ValidateIdleInterval IdleInterval 校验：负数报错，0 合法
func TestWorkerConfig_ValidateIdleInterval(t *testing.T) {
	cfg := newTestWorkerConfig(0)
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected IdleInterval=0 to be valid, got %v", err)
	}

	cfg.IdleInterval = time.Second
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected positive IdleInterval to be valid, got %v", err)
	}

	cfg.IdleInterval = -time.Second
	if err := cfg.Validate(); !errors.Is(err, ErrInvalidIdleInterval) {
		t.Fatalf("expected ErrInvalidIdleInterval, got %v", err)
	}
}

func containsTaskType(types []string, target string) bool {
	for _, t := range types {
		if t == target {
			return true
		}
	}
	return false
}
