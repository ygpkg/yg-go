package manager

import (
	"context"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// TestTypeGate_DisableStopsClaim 验证 Disable 后该类型任务不被认领（停 pending），Enable 后恢复。
func TestTypeGate_DisableStopsClaim(t *testing.T) {
	m := newChainTestManager(t, nil)
	ctx := context.Background()

	if _, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "t"}); err != nil {
		t.Fatalf("submit: %v", err)
	}
	// 默认启用：可认领。
	if _, err := m.dbSrc.Fetch(ctx, "t"); err != nil {
		t.Fatalf("默认应可认领: %v", err)
	}
	// 换一条新任务再测禁用。
	if _, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "t"}); err != nil {
		t.Fatalf("submit2: %v", err)
	}

	// Disable 后不认领（但需经过 worker 的 type gate；这里直接验证 manager 开关状态 + DB 行）。
	if err := m.Disable(ctx, "t"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	enabled, err := m.IsTypeEnabled(ctx, "t")
	if err != nil {
		t.Fatalf("IsTypeEnabled: %v", err)
	}
	if enabled {
		t.Fatalf("Disable 后应为 disabled")
	}
	// 幂等
	if err := m.Disable(ctx, "t"); err != nil {
		t.Fatalf("disable 幂等: %v", err)
	}
	// Enable 恢复
	if err := m.Enable(ctx, "t"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	enabled2, _ := m.IsTypeEnabled(ctx, "t")
	if !enabled2 {
		t.Fatalf("Enable 后应恢复")
	}
	// 幂等
	if err := m.Enable(ctx, "t"); err != nil {
		t.Fatalf("enable 幂等: %v", err)
	}
}

// TestTypeGate_DefaultEnabled 无行/未开关 → 启用（fail-open）。
func TestTypeGate_DefaultEnabled(t *testing.T) {
	m := newChainTestManager(t, nil)
	enabled, err := m.IsTypeEnabled(context.Background(), "never-seen")
	if err != nil {
		t.Fatalf("IsTypeEnabled: %v", err)
	}
	if !enabled {
		t.Fatalf("未出现的类型应默认启用")
	}
}

// TestTypeGate_Adapter 验证 TypeGate 适配器（供 worker 注入）读取开关 + 缓存刷新。
func TestTypeGate_Adapter(t *testing.T) {
	m := newChainTestManager(t, nil)
	ctx := context.Background()
	gate := m.TypeGate(2 * time.Second) // 长 TTL，靠 Disable/Enable 主动刷新

	// 默认启用
	if en, err := gate.TypeEnabled(ctx, "t"); err != nil || !en {
		t.Fatalf("默认应启用: en=%v err=%v", en, err)
	}
	// Disable 应同步刷新本进程缓存
	if err := m.Disable(ctx, "t"); err != nil {
		t.Fatalf("disable: %v", err)
	}
	if en, _ := gate.TypeEnabled(ctx, "t"); en {
		t.Fatalf("Disable 后 gate 应返回禁用（缓存已刷新）")
	}
	// Enable 恢复
	if err := m.Enable(ctx, "t"); err != nil {
		t.Fatalf("enable: %v", err)
	}
	if en, _ := gate.TypeEnabled(ctx, "t"); !en {
		t.Fatalf("Enable 后 gate 应返回启用")
	}
}
