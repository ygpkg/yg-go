package manager_test

// 本期不带 asynq/redis 子包：源 notifier_test.go 依赖
// github.com/hibiken/asynq、github.com/alicebob/miniredis/v2 与 manager/notifier/asynq
// 子包，其通知链路用例（A7 唤醒/降级等）不移植。
// 这里仅保留不依赖任何队列库的纯逻辑用例：队列名拼接与空通知器。

import (
	"context"
	"testing"

	"github.com/ygpkg/yg-go/asynctask/manager"
)

// TestNotifierQueue 队列名 <prefix>:<task_type>；空 prefix 回退默认 gasync。
func TestNotifierQueue(t *testing.T) {
	if got := manager.NotifierQueue("gasync", "invoice:issue"); got != "gasync:invoice:issue" {
		t.Fatalf("NotifierQueue = %q, want gasync:invoice:issue", got)
	}
	if got := manager.NotifierQueue("", "notify:send"); got != "gasync:notify:send" {
		t.Fatalf("NotifierQueue(empty prefix) = %q, want gasync:notify:send", got)
	}
}

// TestNopNotifier 空实现：Notify 恒成功，Consume/Close 空操作。
func TestNopNotifier(t *testing.T) {
	n := manager.NopNotifier()
	if err := n.Notify(context.Background(), "t"); err != nil {
		t.Fatalf("nop Notify: %v", err)
	}
	if err := n.Consume(context.Background(), []string{"t"}, func(context.Context, string) {}); err != nil {
		t.Fatalf("nop Consume: %v", err)
	}
	if err := n.Close(); err != nil {
		t.Fatalf("nop Close: %v", err)
	}
}
