package worker

// 本文件是 worker 包内部测试：直接断言「本地活跃执行表」的匹配规则。
//
// 为什么放在包内：cancelRegistry 是未导出的实现细节（本地活跃执行表），
// 它的对外契约是「按 (任务 id, generation, attempt) 精确取消」，用内部测试断言最直接。

import (
	"context"
	"testing"

	"github.com/ygpkg/yg-go/asynctask"
)

// TestCancelRegistryMatchesExecutionIdentity 取消只命中「同一次执行」。
//
// 这是「晚到取消不影响新执行」的核心：旧 generation/attempt 的通知匹配不到新执行。
func TestCancelRegistryMatchesExecutionIdentity(t *testing.T) {
	reg := newCancelRegistry()
	var canceled []string
	reg.register(cancelKey{taskID: "j1", generation: 1, attempt: 2}, nil,
		func() { canceled = append(canceled, "gen1-attempt2") })
	reg.register(cancelKey{taskID: "j1", generation: 2, attempt: 1}, nil,
		func() { canceled = append(canceled, "gen2-attempt1") })

	// 取消通知指向旧 generation 的那次执行：不得误杀新执行。
	if hit := reg.cancelMatching(asynctask.CancelSignal{TaskID: "j1", Generation: 1, Attempt: 7}); hit != 0 {
		t.Fatalf("attempt 不匹配不该命中，hit = %d", hit)
	}
	if len(canceled) != 0 {
		t.Fatalf("不该取消任何执行，got %v", canceled)
	}

	// 精确命中。
	if hit := reg.cancelMatching(asynctask.CancelSignal{TaskID: "j1", Generation: 2, Attempt: 1}); hit != 1 {
		t.Fatalf("精确匹配应命中 1 次，hit = %d", hit)
	}
	if len(canceled) != 1 || canceled[0] != "gen2-attempt1" {
		t.Fatalf("命中的执行 = %v, want [gen2-attempt1]", canceled)
	}
}

// TestCancelRegistryWildcardSignal 不带 generation/attempt 的信号按任务 id 通配（pending 时取消就是这种）。
func TestCancelRegistryWildcardSignal(t *testing.T) {
	reg := newCancelRegistry()
	hit := 0
	for _, k := range []cancelKey{
		{taskID: "j1", generation: 1, attempt: 1},
		{taskID: "j1", generation: 3, attempt: 5},
		{taskID: "j2", generation: 1, attempt: 1},
	} {
		reg.register(k, nil, func() { hit++ })
	}
	if n := reg.cancelMatching(asynctask.CancelSignal{TaskID: "j1"}); n != 2 {
		t.Fatalf("通配信号应命中同一任务的 2 次执行，hit = %d", n)
	}
	if hit != 2 {
		t.Fatalf("实际取消 %d 次，want 2（不得波及其它任务）", hit)
	}
}

// TestCancelRegistryUnregister 注销后不再命中（执行结束后通知不应再取消任何东西）。
func TestCancelRegistryUnregister(t *testing.T) {
	reg := newCancelRegistry()
	key := cancelKey{taskID: "j1", generation: 1, attempt: 1}
	reg.register(key, nil, func() { t.Fatal("已注销的执行不该被取消") })
	reg.unregister(key)
	if n := reg.cancelMatching(asynctask.CancelSignal{TaskID: "j1"}); n != 0 {
		t.Fatalf("hit = %d, want 0", n)
	}
	if reg.len() != 0 {
		t.Fatalf("在途执行数 = %d, want 0", reg.len())
	}
	_ = context.Background()
}
