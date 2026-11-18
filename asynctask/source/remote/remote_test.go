package remote

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// fakeCenter 是一个可编程的假远程任务中心。
type fakeCenter struct {
	t *testing.T

	mu       sync.Mutex
	handlers map[string]http.HandlerFunc

	// 记录收到的请求体，便于断言。
	lastGetPending map[string]any
	lastHeartbeat  map[string]any
	callbacks      []map[string]any
}

func newFakeCenter(t *testing.T) *fakeCenter {
	f := &fakeCenter{t: t, handlers: map[string]http.HandlerFunc{}}
	return f
}

func (f *fakeCenter) on(path string, h http.HandlerFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[path] = h
}

func (f *fakeCenter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	f.mu.Lock()
	h, ok := f.handlers[path]
	f.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	switch path {
	case "/GetPendingAsyncTask":
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		f.mu.Lock()
		f.lastGetPending = m
		f.mu.Unlock()
	case "/Heartbeat":
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		f.mu.Lock()
		f.lastHeartbeat = m
		f.mu.Unlock()
	case "/Callback":
		var m map[string]any
		_ = json.NewDecoder(r.Body).Decode(&m)
		f.mu.Lock()
		f.callbacks = append(f.callbacks, m)
		f.mu.Unlock()
	}
	h(w, r)
}

func (f *fakeCenter) callbackCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.callbacks)
}

func (f *fakeCenter) lastCallback() map[string]any {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.callbacks) == 0 {
		return nil
	}
	return f.callbacks[len(f.callbacks)-1]
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func newSource(t *testing.T, f *fakeCenter) *Source {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	s, err := New(Config{
		BaseURL:             srv.URL,
		WorkerID:            "w-1",
		CallbackRetryDelays: []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close(context.Background()) })
	return s
}

// ---- Fetch ----

func TestFetch_HasAsyncTask(t *testing.T) {
	f := newFakeCenter(t)
	f.on("/GetPendingAsyncTask", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": true,
			"asyncTask": map[string]any{
				"id": "asyncTask-1", "type": "invoice", "payload": "aGVsbG8=",
				"attempt": 3, "max_attempts": 5, "timeout_ms": 1500,
			},
		})
	})
	s := newSource(t, f)

	asyncTask, err := s.Fetch(context.Background(), "invoice")
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if asyncTask == nil || asyncTask.ID != "asyncTask-1" {
		t.Fatalf("unexpected asyncTask: %+v", asyncTask)
	}
	// attempt 由中心给出。
	if asyncTask.Attempt != 3 {
		t.Errorf("attempt = %d, want 3 (from center)", asyncTask.Attempt)
	}
	if asyncTask.MaxAttempts != 5 {
		t.Errorf("max_attempts = %d, want 5", asyncTask.MaxAttempts)
	}
	if asyncTask.Timeout != 1500*time.Millisecond {
		t.Errorf("timeout = %v, want 1.5s", asyncTask.Timeout)
	}
	if string(asyncTask.Payload) != "hello" {
		t.Errorf("payload = %q, want hello", asyncTask.Payload)
	}
	// 请求体断言。
	f.mu.Lock()
	got := f.lastGetPending
	f.mu.Unlock()
	if got["task_type"] != "invoice" || got["worker_id"] != "w-1" {
		t.Errorf("GetPendingAsyncTask body = %v, want task_type=invoice worker_id=w-1", got)
	}
}

func TestFetch_NoAsyncTask(t *testing.T) {
	for name, status := range map[string]int{
		"available_false": http.StatusOK,
		"no_content":      http.StatusNoContent,
	} {
		t.Run(name, func(t *testing.T) {
			f := newFakeCenter(t)
			f.on("/GetPendingAsyncTask", func(w http.ResponseWriter, _ *http.Request) {
				if status == http.StatusNoContent {
					w.WriteHeader(http.StatusNoContent)
					return
				}
				writeJSON(w, status, map[string]any{"available": false})
			})
			s := newSource(t, f)
			asyncTask, err := s.Fetch(context.Background(), "invoice")
			if asyncTask != nil {
				t.Fatalf("want nil asyncTask, got %+v", asyncTask)
			}
			if !errors.Is(err, asynctask.ErrNoAsyncTask) {
				t.Fatalf("err = %v, want ErrNoAsyncTask", err)
			}
		})
	}
}

// TestFetch_5xx_NotNoAsyncTask 证明「5xx 时 Worker 不空转」：
// 5xx 必须返回一个非 ErrNoAsyncTask 的普通错误，Worker 据此走错误退避而非把 5xx 当「无任务」。
func TestFetch_5xx_NotNoAsyncTask(t *testing.T) {
	f := newFakeCenter(t)
	f.on("/GetPendingAsyncTask", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("center down"))
	})
	s := newSource(t, f)

	asyncTask, err := s.Fetch(context.Background(), "invoice")
	if asyncTask != nil {
		t.Fatalf("want nil asyncTask, got %+v", asyncTask)
	}
	if err == nil {
		t.Fatal("want error on 5xx, got nil")
	}
	if errors.Is(err, asynctask.ErrNoAsyncTask) {
		t.Fatalf("5xx must NOT map to ErrNoAsyncTask (would cause busy-spin), got %v", err)
	}
	if !strings.Contains(err.Error(), "502") {
		t.Errorf("error should carry status: %v", err)
	}
}

// ---- Renew ----

func TestRenew_Success(t *testing.T) {
	f := newFakeCenter(t)
	f.on("/Heartbeat", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	s := newSource(t, f)

	if err := s.Renew(context.Background(), &asynctask.AsyncTask{ID: "asyncTask-1"}); err != nil {
		t.Fatalf("Renew: %v", err)
	}
	f.mu.Lock()
	got := f.lastHeartbeat
	f.mu.Unlock()
	if got["task_id"] != "asyncTask-1" || got["worker_id"] != "w-1" {
		t.Errorf("Heartbeat body = %v", got)
	}
}

// TestRenew_TwoConsecutiveFailures 验证：第 1 次失败返回普通错误（非 ErrRenewLost），
// 第 2 次「连续」失败返回 ErrRenewLost（Worker 侧据此取消执行 ctx）。
func TestRenew_TwoConsecutiveFailures(t *testing.T) {
	f := newFakeCenter(t)
	f.on("/Heartbeat", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	s := newSource(t, f)
	asyncTask := &asynctask.AsyncTask{ID: "asyncTask-1"}

	err1 := s.Renew(context.Background(), asyncTask)
	if err1 == nil {
		t.Fatal("1st renew want error, got nil")
	}
	if errors.Is(err1, ErrRenewLost) {
		t.Fatalf("1st failure must not be ErrRenewLost yet: %v", err1)
	}

	err2 := s.Renew(context.Background(), asyncTask)
	if err2 == nil {
		t.Fatal("2nd renew want error, got nil")
	}
	if !errors.Is(err2, ErrRenewLost) {
		t.Fatalf("2nd consecutive failure must be ErrRenewLost, got %v", err2)
	}
}

// TestRenew_SuccessResetsCounter 验证连续失败计数在成功后被清零。
func TestRenew_SuccessResetsCounter(t *testing.T) {
	f := newFakeCenter(t)
	var fail atomic.Bool
	fail.Store(true)
	f.on("/Heartbeat", func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	s := newSource(t, f)
	asyncTask := &asynctask.AsyncTask{ID: "asyncTask-1"}

	_ = s.Renew(context.Background(), asyncTask) // fail 1
	fail.Store(false)
	if err := s.Renew(context.Background(), asyncTask); err != nil { // success resets
		t.Fatalf("renew success: %v", err)
	}
	fail.Store(true)
	err := s.Renew(context.Background(), asyncTask) // fail again -> only 1st consecutive
	if err == nil {
		t.Fatal("want error")
	}
	if errors.Is(err, ErrRenewLost) {
		t.Fatalf("after a success the counter must reset; got ErrRenewLost: %v", err)
	}
}

// ---- Save ----

// TestSave_Success_CallbackAndPost 验证：
//   - 先调本地钩子，再 POST Callback（顺序）；
//   - 远程形态 asynctask.TxFromContext(ctx) == nil；
//   - 回调 carry 正确的 status / result / worker_id / task_id。
func TestSave_Success_CallbackAndPost(t *testing.T) {
	f := newFakeCenter(t)
	f.on("/Callback", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	s := newSource(t, f)

	var order []string
	var txSeen bool
	callback := func(ctx context.Context) error {
		order = append(order, "hook")
		txSeen = asynctask.TxFromContext(ctx) != nil // 远程形态必须为 nil
		return nil
	}

	r := asynctask.Result{Outcome: asynctask.OutcomeSucceeded, Payload: []byte("done")}
	if err := s.Save(context.Background(), &asynctask.AsyncTask{ID: "asyncTask-1"}, r, callback); err != nil {
		t.Fatalf("Save: %v", err)
	}

	// 钩子先于 Callback。
	if len(f.callbacks) == 0 {
		order = append(order, "no-callback")
	} else if order[0] == "hook" {
		order = append(order, "callback")
	}
	if strings.Join(order, ">") != "hook>callback" {
		t.Fatalf("order = %v, want hook>callback", order)
	}
	if txSeen {
		t.Fatal("TxFromContext must be nil in remote Save hook context")
	}

	cb := f.lastCallback()
	if cb["task_id"] != "asyncTask-1" || cb["worker_id"] != "w-1" || cb["status"] != "succeeded" {
		t.Errorf("callback body = %v", cb)
	}
}

// TestSave_FailureStillCallbacks 验证失败也回调（status=failed + error 上报），否则中心会一直认为任务在跑。
func TestSave_FailureStillCallbacks(t *testing.T) {
	f := newFakeCenter(t)
	f.on("/Callback", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	s := newSource(t, f)

	r := asynctask.Result{Outcome: asynctask.OutcomeFailed, Failure: asynctask.FailureHandler, Err: errors.New("boom")}
	if err := s.Save(context.Background(), &asynctask.AsyncTask{ID: "asyncTask-1"}, r, nil); err != nil {
		t.Fatalf("Save: %v", err)
	}
	cb := f.lastCallback()
	if cb["status"] != "failed" {
		t.Errorf("status = %v, want failed", cb["status"])
	}
	if s, _ := cb["error"].(string); !strings.Contains(s, "boom") {
		t.Errorf("error field should carry handler error, got %q", s)
	}
}

// TestSave_HookErrorDoesNotBlockCallback 钩子报错仍要回传，且被吞为 nil 返回。
func TestSave_HookErrorDoesNotBlockCallback(t *testing.T) {
	f := newFakeCenter(t)
	f.on("/Callback", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	s := newSource(t, f)

	callback := func(context.Context) error { return errors.New("hook failed") }
	r := asynctask.Result{Outcome: asynctask.OutcomeSucceeded, Payload: []byte("ok")}
	if err := s.Save(context.Background(), &asynctask.AsyncTask{ID: "asyncTask-1"}, r, callback); err != nil {
		t.Fatalf("Save should not fail on hook error (still callback), got %v", err)
	}
	if f.callbackCount() != 1 {
		t.Fatalf("callback count = %d, want 1", f.callbackCount())
	}
}

// TestSave_CallbackRetry 验证 Callback 失败按配置重试，最终成功后返回 nil。
func TestSave_CallbackRetry(t *testing.T) {
	f := newFakeCenter(t)
	var hits int32
	f.on("/Callback", func(w http.ResponseWriter, _ *http.Request) {
		n := atomic.AddInt32(&hits, 1)
		if n < 3 { // 前两次失败，第三次成功
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	s := newSource(t, f)

	r := asynctask.Result{Outcome: asynctask.OutcomeSucceeded}
	if err := s.Save(context.Background(), &asynctask.AsyncTask{ID: "asyncTask-1"}, r, nil); err != nil {
		t.Fatalf("Save should recover after retries: %v", err)
	}
	if got := atomic.LoadInt32(&hits); got != 3 {
		t.Fatalf("callback hits = %d, want 3 (2 fail + 1 success)", got)
	}
}

// TestSave_CallbackExhausted 验证重试耗尽后返回 ErrCallbackLost（接入方兜底）。
func TestSave_CallbackExhausted(t *testing.T) {
	f := newFakeCenter(t)
	var hits int32
	f.on("/Callback", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.WriteHeader(http.StatusInternalServerError)
	})
	s := newSource(t, f)

	err := s.Save(context.Background(), &asynctask.AsyncTask{ID: "asyncTask-1"}, asynctask.Result{Outcome: asynctask.OutcomeSucceeded}, nil)
	if !errors.Is(err, ErrCallbackLost) {
		t.Fatalf("err = %v, want ErrCallbackLost", err)
	}
	// 首次 + 3 次重试 = 4 次。
	if got := atomic.LoadInt32(&hits); got != 4 {
		t.Fatalf("callback hits = %d, want 4 (1+3 retries)", got)
	}
}

// ---- NoAsyncTaskBackoff ----

func TestNoAsyncTaskBackoff(t *testing.T) {
	cases := []struct {
		n    int
		want time.Duration
	}{
		{0, 500 * time.Millisecond},
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 5 * time.Second},
		{10, 5 * time.Second},
		{-1, 500 * time.Millisecond},
	}
	for _, c := range cases {
		if got := NoAsyncTaskBackoff(c.n); got != c.want {
			t.Errorf("NoAsyncTaskBackoff(%d) = %v, want %v", c.n, got, c.want)
		}
	}
}

// ---- Close ----

func TestClose(t *testing.T) {
	s, err := New(Config{BaseURL: "http://127.0.0.1:1", WorkerID: "w-1"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Close(context.Background()); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

// 断言 *Source 实现了 asynctask.Source 接口。
var _ asynctask.Source = (*Source)(nil)
