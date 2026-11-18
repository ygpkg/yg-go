package worker_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// Start 必须立即返回（不阻塞等待槽位退出），且启动后 Run 仍能正常收尾。
func TestStart_ReturnsImmediately(t *testing.T) {
	src := newMemSource()
	w := worker.New(src, &worker.WorkerConfig{
		Concurrency:  2,
		PollInterval: 5 * time.Millisecond,
	})
	if err := w.Register("invoice:issue", worker.HandlerFunc(func(context.Context, *asynctask.AsyncTask) error { return nil })); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- w.Start(ctx) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Start blocked; must return immediately")
	}

	// 停机并确认可优雅退出（槽位真实起来了）。
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	if err := w.Shutdown(stopCtx); err != nil {
		t.Fatalf("shutdown: %v", err)
	}
	cancel()
}

// 未注册类型的 Start 返回 ErrNoType。
func TestStart_NoType(t *testing.T) {
	w := worker.New(newMemSource(), nil)
	if err := w.Start(context.Background()); !errors.Is(err, asynctask.ErrNoType) {
		t.Fatalf("want ErrNoType, got %v", err)
	}
}

// src 为 nil 的 Start 返回 ErrInvalidConfig。
func TestStart_InvalidConfig(t *testing.T) {
	w := worker.New(nil, nil)
	if err := w.Start(context.Background()); !errors.Is(err, asynctask.ErrInvalidConfig) {
		t.Fatalf("want ErrInvalidConfig, got %v", err)
	}
}

// 重复 Start（以及 Run 后再 Start）返回 ErrWorkerAlreadyRunning。
func TestStart_AlreadyRunning(t *testing.T) {
	w := worker.New(newMemSource(), &worker.WorkerConfig{PollInterval: 5 * time.Millisecond})
	if err := w.Register("a", worker.HandlerFunc(func(context.Context, *asynctask.AsyncTask) error { return nil })); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := w.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	if err := w.Start(context.Background()); !errors.Is(err, asynctask.ErrWorkerAlreadyRunning) {
		t.Fatalf("want ErrWorkerAlreadyRunning on double Start, got %v", err)
	}
	// Run 后再 Start 同样应报 ErrWorkerAlreadyRunning。
	if err := w.Run(context.Background()); !errors.Is(err, asynctask.ErrWorkerAlreadyRunning) {
		t.Fatalf("want ErrWorkerAlreadyRunning on Run after Start, got %v", err)
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
}

// 停止后 Start 返回 ErrWorkerStopped。
func TestStart_Stopped(t *testing.T) {
	w := worker.New(newMemSource(), nil)
	if err := w.Register("a", worker.HandlerFunc(func(context.Context, *asynctask.AsyncTask) error { return nil })); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := w.Shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown before start: %v", err)
	}
	if err := w.Start(context.Background()); !errors.Is(err, asynctask.ErrWorkerStopped) {
		t.Fatalf("want ErrWorkerStopped, got %v", err)
	}
}

// Start 之后 Run 应阻塞至 ctx 取消并返回 ctx.Err()。
func TestStart_RunBlocksUntilCancel(t *testing.T) {
	src := newMemSource()
	w := worker.New(src, &worker.WorkerConfig{PollInterval: 5 * time.Millisecond})
	if err := w.Register("a", worker.HandlerFunc(func(context.Context, *asynctask.AsyncTask) error { return nil })); err != nil {
		t.Fatalf("register: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	select {
	case <-done:
		t.Fatal("Run returned before cancel/Shutdown; must block")
	case <-time.After(50 * time.Millisecond):
	}

	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()
	_ = w.Shutdown(stopCtx)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after cancel")
	}
}
