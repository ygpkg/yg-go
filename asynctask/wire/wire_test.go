package wire

import (
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

func TestFromToAsyncTask(t *testing.T) {
	src := &asynctask.AsyncTask{
		ID:          "id-1",
		Type:        "email:send",
		Priority:    7,
		Payload:     []byte(`{"to":"a@b.c"}`),
		Attempt:     2,
		MaxAttempts: 5,
		Timeout:     90 * time.Second,
		RequestID:   "req-1",
		TraceID:     "trace-1",
	}
	w := FromAsyncTask(src)
	if w == nil {
		t.Fatal("FromAsyncTask returned nil")
	}
	if w.TimeoutMS != 90000 {
		t.Fatalf("timeout_ms want 90000 got %d", w.TimeoutMS)
	}
	back := w.ToAsyncTask()
	if back.Status != asynctask.StatusRunning {
		t.Fatalf("status want running got %s", back.Status)
	}
	if back.Timeout != 90*time.Second {
		t.Fatalf("timeout roundtrip failed: %v", back.Timeout)
	}
	if back.ID != src.ID || back.Type != src.Type || back.Attempt != src.Attempt {
		t.Fatalf("roundtrip field mismatch: %+v", back)
	}

	if FromAsyncTask(nil) != nil {
		t.Fatal("FromAsyncTask(nil) should be nil")
	}
	var nilTask *Task
	if nilTask.ToAsyncTask() != nil {
		t.Fatal("nil Task.ToAsyncTask should be nil")
	}
}

func TestParseOutcome(t *testing.T) {
	ok := []string{"succeeded", "failed", "skipped", "canceled"}
	for _, s := range ok {
		if _, valid := ParseOutcome(s); !valid {
			t.Fatalf("%s should parse", s)
		}
	}
	if _, valid := ParseOutcome("bogus"); valid {
		t.Fatal("bogus should not parse")
	}
}
