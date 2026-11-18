package manager

import (
	"context"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// TestP4_TimeoutStatus 验证超时任务落 StatusTimeout 且 Stats 计数正确。
func TestP4_TimeoutStatus(t *testing.T) {
	m := newChainTestManager(t, nil)
	ctx := context.Background()
	// MaxAttempts=1：本次失败即终态（超时应落 StatusTimeout）。
	j, _, err := m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "t", MaxAttempts: 1})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	// 认领后以「超时」结果 Save。
	claimed, err := m.dbSrc.Fetch(ctx, "t")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	_ = j
	if err := m.dbSrc.Save(ctx, claimed, asynctask.Result{
		Outcome: asynctask.OutcomeFailed,
		Failure: asynctask.FailureTimeout,
	}, nil); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, _ := m.Get(ctx, claimed.ID)
	if got.Status != asynctask.StatusTimeout {
		t.Fatalf("超时应落 status=timeout，实际 %s", got.Status)
	}
	st, _ := m.Stats(ctx)
	if st.Timeout != 1 {
		t.Fatalf("Stats.Timeout=%d, 期望 1", st.Timeout)
	}
}

// TestP4_LabelsFilter 验证按 label 精确过滤。
func TestP4_LabelsFilter(t *testing.T) {
	m := newChainTestManager(t, nil)
	ctx := context.Background()
	_, _, _ = m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "t", Labels: map[string]string{"app_group": "X", "company_id": "1"}})
	_, _, _ = m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "t", Labels: map[string]string{"app_group": "Y", "company_id": "1"}})
	_, _, _ = m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "t"})

	list, total, err := m.List(ctx, &asynctask.AsyncTaskCond{Labels: map[string]string{"app_group": "X"}})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if total != 1 || len(list) != 1 {
		t.Fatalf("按 app_group=X 过滤 total=%d len=%d, 期望 1/1", total, len(list))
	}
	if list[0].Labels["company_id"] != "1" {
		t.Fatalf("labels 回读错: %+v", list[0].Labels)
	}

	// 多 label AND
	_, total2, _ := m.List(ctx, &asynctask.AsyncTaskCond{Labels: map[string]string{"app_group": "Y", "company_id": "1"}})
	if total2 != 1 {
		t.Fatalf("AND 过滤 total=%d, 期望 1", total2)
	}
}

// TestP4_OrderByWhitelist 验证 OrderBy 白名单（非法值回退默认，不报错注入）。
func TestP4_OrderByWhitelist(t *testing.T) {
	m := newChainTestManager(t, nil)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_, _, _ = m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "t"})
	}
	// 合法的 priority asc
	if _, _, err := m.List(ctx, &asynctask.AsyncTaskCond{OrderBy: "priority asc"}); err != nil {
		t.Fatalf("orderBy priority asc: %v", err)
	}
	// 非法值不应导致 SQL 错误（回退默认）
	if _, _, err := m.List(ctx, &asynctask.AsyncTaskCond{OrderBy: "created_at; DROP TABLE core_async_task"}); err != nil {
		t.Fatalf("非法 orderBy 应回退默认而非报错: %v", err)
	}
}

// TestP4_TimeRange 验证创建时间区间过滤。
func TestP4_TimeRange(t *testing.T) {
	m := newChainTestManager(t, nil)
	ctx := context.Background()
	_, _, _ = m.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "t"})
	now := time.Now()
	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	// 全区间命中
	_, total, _ := m.List(ctx, &asynctask.AsyncTaskCond{CreatedAfter: &past, CreatedBefore: &future})
	if total != 1 {
		t.Fatalf("区间内 total=%d, 期望 1", total)
	}
	// 排除区间
	_, total2, _ := m.List(ctx, &asynctask.AsyncTaskCond{CreatedAfter: &future})
	if total2 != 0 {
		t.Fatalf("未来区间 total=%d, 期望 0", total2)
	}
}
