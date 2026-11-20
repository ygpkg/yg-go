package db

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/gorm"
)

// newResultLimitSource 构造一个指定 MaxResultBytes 的 Source（sqlite 内存库，已建表）。
func newResultLimitSource(t *testing.T, maxResultBytes int) (*source, *gorm.DB) {
	t.Helper()
	gdb := openTestDB(t)
	src, err := New(Config{
		DB:             gdb,
		LeaseTTL:       60 * time.Second,
		WorkerID:       "w-result",
		MaxResultBytes: maxResultBytes,
	})
	if err != nil {
		t.Fatalf("new source: %v", err)
	}
	return src, gdb
}

// TestNewMaxResultBytesDefault 不传 MaxResultBytes 时退回默认 4096（兼容历史行为）。
func TestNewMaxResultBytesDefault(t *testing.T) {
	src, _ := newResultLimitSource(t, 0)
	if src.maxResultBytes != DefaultMaxResultBytes {
		t.Fatalf("maxResultBytes = %d, want 默认 %d", src.maxResultBytes, DefaultMaxResultBytes)
	}
	if DefaultMaxResultBytes != 4096 {
		t.Fatalf("DefaultMaxResultBytes = %d, want 4096（口径不得漂移）", DefaultMaxResultBytes)
	}
}

// TestResultTextHonoursConfig 结果快照按配置上限截断，不再写死 4096。
func TestResultTextHonoursConfig(t *testing.T) {
	src, _ := newResultLimitSource(t, 16<<10)

	payload := []byte(strings.Repeat("x", 20<<10))
	got, ok := src.resultText(payload).(string)
	if !ok {
		t.Fatalf("resultText 非空 payload 应返回 string")
	}
	if len(got) != 16<<10 {
		t.Fatalf("截断长度 = %d, want %d", len(got), 16<<10)
	}

	// 未超上限不截断。
	small := []byte(`{"ok":true}`)
	if got := src.resultText(small).(string); got != string(small) {
		t.Fatalf("未超上限的结果被改写：%q", got)
	}

	// 空结果仍写 NULL（nil），不落空串。
	if got := src.resultText(nil); got != nil {
		t.Fatalf("空 payload resultText = %v, want nil", got)
	}
}

// TestSaveResultTruncatedPerConfig 落库端到端：同一份 8 KiB 结果，默认配置截到 4096，
// 调大后完整写入——任务行与尝试行两处 result 用同一个上限。
func TestSaveResultTruncatedPerConfig(t *testing.T) {
	cases := []struct {
		name       string
		maxBytes   int
		wantStored int
	}{
		{name: "默认 4KiB 截断", maxBytes: 0, wantStored: DefaultMaxResultBytes},
		{name: "16KiB 完整写入", maxBytes: 16 << 10, wantStored: 8 << 10},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			src, gdb := newResultLimitSource(t, c.maxBytes)
			ctx := context.Background()
			id := seedPending(t, gdb, "result", 3)

			task, err := src.Fetch(ctx, "result")
			if err != nil {
				t.Fatalf("fetch: %v", err)
			}
			payload := []byte(strings.Repeat("r", 8<<10))
			if err := src.Save(ctx, task, asynctask.Result{
				Outcome:  asynctask.OutcomeSucceeded,
				Duration: time.Millisecond,
				Payload:  payload,
			}, nil); err != nil {
				t.Fatalf("save: %v", err)
			}

			var taskResult, runResult *string
			if err := gdb.Raw(`SELECT result FROM core_async_task WHERE id = ?`, id).
				Scan(&taskResult).Error; err != nil {
				t.Fatalf("读任务 result: %v", err)
			}
			if err := gdb.Raw(`SELECT result FROM core_async_task_run WHERE async_task_id = ?`, id).
				Scan(&runResult).Error; err != nil {
				t.Fatalf("读尝试 result: %v", err)
			}
			for name, got := range map[string]*string{"core_async_task.result": taskResult, "run.result": runResult} {
				if got == nil {
					t.Fatalf("%s 为空，want %d 字节", name, c.wantStored)
				}
				if len(*got) != c.wantStored {
					t.Fatalf("%s 长度 = %d, want %d", name, len(*got), c.wantStored)
				}
			}
		})
	}
}
