package asynctask

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestBindEmptyPayload(t *testing.T) {
	var v map[string]any
	if err := (&AsyncTask{}).Bind(&v); err != nil {
		t.Fatalf("empty payload should return nil, got %v", err)
	}
	if v != nil {
		t.Fatalf("expected nil map, got %v", v)
	}
}

func TestBindValidPayload(t *testing.T) {
	task := &AsyncTask{Payload: []byte(`{"a":1}`)}
	var v struct{ A int }
	if err := task.Bind(&v); err != nil {
		t.Fatalf("bind: %v", err)
	}
	if v.A != 1 {
		t.Fatalf("expected A=1, got %d", v.A)
	}
}

func TestIsStuck(t *testing.T) {
	now := time.Now()
	past := now.Add(-time.Minute)
	future := now.Add(time.Minute)

	if (&AsyncTask{Status: StatusPending, LeaseUntil: &past}).IsStuck(now) {
		t.Fatal("pending should not be stuck")
	}
	if (&AsyncTask{Status: StatusRunning}).IsStuck(now) {
		t.Fatal("running without lease should not be stuck")
	}
	if !(&AsyncTask{Status: StatusRunning, LeaseUntil: &past}).IsStuck(now) {
		t.Fatal("running with expired lease should be stuck")
	}
	if (&AsyncTask{Status: StatusRunning, LeaseUntil: &future}).IsStuck(now) {
		t.Fatal("running with live lease should not be stuck")
	}
}

func TestCanCancel(t *testing.T) {
	for _, st := range []Status{StatusPending, StatusRunning} {
		if !(&AsyncTask{Status: st}).CanCancel() {
			t.Fatalf("%s should be cancelable", st)
		}
	}
	for _, st := range []Status{StatusSucceeded, StatusFailed, StatusCanceled, StatusSkipped, StatusTimeout} {
		if (&AsyncTask{Status: st}).CanCancel() {
			t.Fatalf("%s should not be cancelable", st)
		}
	}
}

func TestHasRemainingAttempts(t *testing.T) {
	if !(&AsyncTask{MaxAttempts: 0, FailureCount: 99}).HasRemainingAttempts() {
		t.Fatal("MaxAttempts<=0 means unlimited")
	}
	// 判断依据是 FailureCount（失败额度）而不是 Attempt（执行序号）：
	// attempt 很大但没失败过，仍然「有剩余」——暂停不消耗额度。
	if (&AsyncTask{MaxAttempts: 3, Attempt: 9, FailureCount: 2}).HasRemainingAttempts() {
		t.Fatal("本次是第 3 次失败，额度用尽，不该再重试")
	}
	if !(&AsyncTask{MaxAttempts: 3, Attempt: 9, FailureCount: 1}).HasRemainingAttempts() {
		t.Fatal("本次是第 2 次失败，仍有额度")
	}
	if !(&AsyncTask{MaxAttempts: 3, Attempt: 5}).HasRemainingAttempts() {
		t.Fatal("attempt 不影响失败额度：从未失败时仍有额度")
	}
}

func TestClaimable(t *testing.T) {
	if !(&AsyncTask{MaxAttempts: 0, FailureCount: 99}).Claimable() {
		t.Fatal("MaxAttempts<=0 means unlimited")
	}
	if (&AsyncTask{MaxAttempts: 2, FailureCount: 2}).Claimable() {
		t.Fatal("failure_count==max 不该再被认领")
	}
	if !(&AsyncTask{MaxAttempts: 2, FailureCount: 1, Attempt: 7}).Claimable() {
		t.Fatal("暂停后 attempt>max 仍可认领")
	}
}

func TestWithTxRoundTrip(t *testing.T) {
	type fakeTx struct{ name string }
	ctx := WithTx(context.Background(), &fakeTx{name: "tx"})
	got, ok := TxFromContext(ctx).(*fakeTx)
	if !ok || got.name != "tx" {
		t.Fatalf("tx roundtrip failed: %v", TxFromContext(ctx))
	}
	if TxFromContext(context.Background()) != nil {
		t.Fatal("no tx in fresh ctx")
	}
	//nolint:staticcheck // 故意传 nil ctx 验证防御分支
	if TxFromContext(nil) != nil {
		t.Fatal("nil ctx should return nil")
	}
}

func TestErrorSentinelsPrefixed(t *testing.T) {
	errs := []error{
		ErrNilDB, ErrEmptyType, ErrNilHandler, ErrDuplicateType, ErrAsyncTaskNotFound,
		ErrRunNotFound, ErrNotReplayable, ErrPayloadTooLarge, ErrInvalidConfig,
		ErrWorkerAlreadyRunning, ErrWorkerStopped, ErrNoType, ErrUniqKeyRace,
		ErrLeaseLost, ErrNotify, ErrNoAsyncTask,
	}
	for _, e := range errs {
		if e == nil {
			t.Fatal("nil sentinel")
		}
		if len(e.Error()) < len("asynctask: ") || e.Error()[:len("asynctask: ")] != "asynctask: " {
			t.Fatalf("sentinel %v missing asynctask: prefix", e)
		}
	}
	if !errors.Is(ErrLeaseLost, ErrLeaseLost) {
		t.Fatal("errors.Is should match sentinel")
	}
}

func TestLoggerDefaultAndWith(t *testing.T) {
	l := DefaultLogger()
	if l == nil {
		t.Fatal("DefaultLogger nil")
	}
	// 只验证可安全调用（zap 默认 logger）；不校验输出。
	l.Infow(context.Background(), "test", "k", "v")
	l.With("task.type", "x").Warnw(context.Background(), "with-fields")
	if newAsyncTaskLogger("id-1", "type-1") == nil {
		t.Fatal("newAsyncTaskLogger nil")
	}
	if newAsyncTaskLogger("", "type-1") == nil {
		t.Fatal("newAsyncTaskLogger(empty id) nil")
	}
}
