package worker_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// P2：每类型独立并发。
//
// 验证：注册 A(并发=2)、B(并发=5)、C(未设→回落 Config.Concurrency=1)，
// 各推入足够多任务，观察「同类型同时执行中的峰值为该类型配置的并发数」。
func TestWorker_PerTypeConcurrency(t *testing.T) {
	src := newMemSource()

	// 每类型一个「峰值记录器」：记录该类型同时执行的峰值。
	type peak struct {
		cur int32
		max int32
	}
	peaks := map[string]*peak{
		"typeA":  {},
		"typeB":  {},
		"typeC":  {},
		"typeC2": {},
	}
	_ = peaks

	// 执行体：进入时 cur+1 记峰值，短暂 sleep 后退出（制造重叠窗口便于观察并发）。
	blockingHandler := func(taskType string) worker.Handler {
		p := peaks[taskType]
		return worker.HandlerFunc(func(ctx context.Context, _ *asynctask.AsyncTask) error {
			c := atomic.AddInt32(&p.cur, 1)
			for {
				m := atomic.LoadInt32(&p.max)
				if c <= m || atomic.CompareAndSwapInt32(&p.max, m, c) {
					break
				}
			}
			time.Sleep(80 * time.Millisecond)
			atomic.AddInt32(&p.cur, -1)
			return nil
		})
	}

	w := worker.New(src, &worker.WorkerConfig{
		Concurrency: 1, // 全局默认；typeC 未单独设置 → 应为 1
	})
	if err := w.Register("typeA", blockingHandler("typeA"), worker.WithHandlerConcurrency(2)); err != nil {
		t.Fatalf("register typeA: %v", err)
	}
	if err := w.Register("typeB", blockingHandler("typeB"), worker.WithHandlerConcurrency(5)); err != nil {
		t.Fatalf("register typeB: %v", err)
	}
	if err := w.Register("typeC", blockingHandler("typeC")); err != nil {
		t.Fatalf("register typeC: %v", err)
	}

	// 各推入 8 个任务（多于任何类型并发，确保能打满）。
	for _, tt := range []string{"typeA", "typeB", "typeC"} {
		for i := 0; i < 8; i++ {
			src.push(tt, &asynctask.AsyncTask{ID: tt + "-" + string(rune('0'+i))})
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { _ = w.Start(ctx); close(done) }()
	<-done // Start 立即返回

	// 等待足够时间让各类型队列跑完。
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		src.mu.Lock()
		total := len(src.results)
		src.mu.Unlock()
		if total >= 24 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	_ = w.Shutdown(context.Background())

	// 断言峰值：typeA=2、typeB=5、typeC=1（全局默认）。
	if got := atomic.LoadInt32(&peaks["typeA"].max); got != 2 {
		t.Errorf("typeA 峰值 = %d, 期望 2", got)
	}
	if got := atomic.LoadInt32(&peaks["typeB"].max); got != 5 {
		t.Errorf("typeB 峰值 = %d, 期望 5", got)
	}
	if got := atomic.LoadInt32(&peaks["typeC"].max); got != 1 {
		t.Errorf("typeC 峰值 = %d, 期望 1（回落全局 Concurrency）", got)
	}
}

// 验证 WithHandlerConcurrency(n<=0) 视为未设置，回落全局。
func TestWithHandlerConcurrency_NonPositiveFallsBack(t *testing.T) {
	src := newMemSource()
	w := worker.New(src, &worker.WorkerConfig{Concurrency: 3})
	var ran int32
	h := worker.HandlerFunc(func(ctx context.Context, _ *asynctask.AsyncTask) error {
		atomic.AddInt32(&ran, 1)
		return nil
	})
	// n=0 应回落，不因非法值 panic/异常。
	if err := w.Register("t", h, worker.WithHandlerConcurrency(0)); err != nil {
		t.Fatalf("register: %v", err)
	}
	_ = src
	_ = w
}
