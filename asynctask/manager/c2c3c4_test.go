package manager

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"
)

// 本文件覆盖 C3（活跃期唯一）与 C2/C4（一等维度 + 查询入口）的 sqlite 语义。
// 跨方言的 C3 三断言见 reallib_test.go（sqlite/PG/MySQL 各跑一遍）。

// submitKeyed 以固定幂等键提交一条任务。
func submitKeyed(t *testing.T, m *Manager, key string) (*asynctask.AsyncTask, bool) {
	t.Helper()
	task, submitted, err := m.Submit(context.Background(), &asynctask.AsyncTaskSpec{Type: "t", UniqKey: key})
	if err != nil {
		t.Fatalf("submit %q: %v", key, err)
	}
	return task, submitted
}

// TestC3ActiveWindowUniq C3 三条断言：终态后同键可重建、活跃期命中既有行。
func TestC3ActiveWindowUniq(t *testing.T) {
	m := newTestManager(t)

	// 1. 同键已有 failed 行 → Submit 新建（submitted=true 且是新 id），不是返回旧任务。
	j1, s1 := submitKeyed(t, m, "k-failed")
	if !s1 {
		t.Fatalf("首次提交应为新建")
	}
	forceStatus(t, m, j1.ID, asynctask.StatusFailed)
	j2, s2 := submitKeyed(t, m, "k-failed")
	if !s2 {
		t.Fatalf("终态后同键重建：submitted=false，期望 true")
	}
	if j2.ID == j1.ID {
		t.Fatalf("终态后重建应返回新任务，实际返回旧 id %s", j1.ID)
	}

	// 2. 同键已有 pending 行 → 命中既有行（submitted=false、同一 id）。
	j3, _ := submitKeyed(t, m, "k-pending")
	j4, s4 := submitKeyed(t, m, "k-pending")
	if s4 {
		t.Fatalf("活跃期同键提交：submitted=true，期望 false")
	}
	if j4.ID != j3.ID {
		t.Fatalf("活跃期同键提交应返回既有 id %s，实际 %s", j3.ID, j4.ID)
	}

	// 3. 同键行 running → 置 succeeded（终态）→ Submit 新建。
	j5, _ := submitKeyed(t, m, "k-running")
	forceStatus(t, m, j5.ID, asynctask.StatusRunning)
	forceStatus(t, m, j5.ID, asynctask.StatusSucceeded)
	j6, s6 := submitKeyed(t, m, "k-running")
	if !s6 || j6.ID == j5.ID {
		t.Fatalf("running 转终态后同键重建：submitted=%v sameID=%v", s6, j6.ID == j5.ID)
	}

	// 库里 k-failed 应有 2 行（旧 failed + 新 pending），k-pending 只有 1 行。
	for key, want := range map[string]int64{"k-failed": 2, "k-pending": 1, "k-running": 2} {
		var cnt int64
		if err := m.db.Model(&sourcedb.CoreAsyncTask{}).Where("uniq_key = ?", key).Count(&cnt).Error; err != nil {
			t.Fatalf("count %s: %v", key, err)
		}
		if cnt != want {
			t.Fatalf("uniq_key=%s 行数=%d，期望 %d", key, cnt, want)
		}
	}
}

// TestC3GetByUniqKey Active 期回查：只返回活跃行，终态后返回 ErrAsyncTaskNotFound。
func TestC3GetByUniqKey(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	task, _ := submitKeyed(t, m, "k-get")
	got, err := m.GetByUniqKey(ctx, "k-get")
	if err != nil {
		t.Fatalf("GetByUniqKey: %v", err)
	}
	if got.ID != task.ID {
		t.Fatalf("GetByUniqKey 返回 %s，期望 %s", got.ID, task.ID)
	}

	forceStatus(t, m, task.ID, asynctask.StatusSucceeded)
	if _, err := m.GetByUniqKey(ctx, "k-get"); !errors.Is(err, asynctask.ErrAsyncTaskNotFound) {
		t.Fatalf("终态后 GetByUniqKey err = %v，期望 ErrAsyncTaskNotFound", err)
	}
	if _, err := m.GetByUniqKey(ctx, ""); !errors.Is(err, asynctask.ErrAsyncTaskNotFound) {
		t.Fatalf("空键 GetByUniqKey err = %v，期望 ErrAsyncTaskNotFound", err)
	}
}

// submitDim 提交带租户/主体维度的任务。
func submitDim(t *testing.T, m *Manager, companyID uint64, kind, id, taskType string) *asynctask.AsyncTask {
	t.Helper()
	task, submitted, err := m.Submit(context.Background(), &asynctask.AsyncTaskSpec{
		Type: taskType, CompanyID: companyID, SubjectKind: kind, SubjectID: id,
	})
	if err != nil || !submitted {
		t.Fatalf("submit dim: submitted=%v err=%v", submitted, err)
	}
	return task
}

// TestC2DimensionsPersistAndFilter 三列落库、回读、按租户/主体过滤（含 company_id=0 是合法值）。
func TestC2DimensionsPersistAndFilter(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	t0 := submitDim(t, m, 0, "work", "w-0", "gen")
	t7 := submitDim(t, m, 7, "work", "w-7", "gen")
	submitDim(t, m, 7, "file", "f-7", "upload")

	// 回读字段。
	got, err := m.Get(ctx, t7.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.CompanyID != 7 || got.SubjectKind != "work" || got.SubjectID != "w-7" {
		t.Fatalf("维度回读错：%d/%q/%q", got.CompanyID, got.SubjectKind, got.SubjectID)
	}
	// 落的是独立列（不是 labels）。
	var row sourcedb.CoreAsyncTask
	if err := m.db.Where("id = ?", t7.ID).Take(&row).Error; err != nil {
		t.Fatalf("load row: %v", err)
	}
	if row.CompanyID != 7 || row.SubjectKind != "work" || row.SubjectID != "w-7" {
		t.Fatalf("维度列未落库：%+v", row)
	}

	zero := uint64(0)
	rows, total, err := m.List(ctx, &asynctask.AsyncTaskCond{CompanyID: &zero})
	if err != nil {
		t.Fatalf("List(company=0): %v", err)
	}
	if total != 1 || len(rows) != 1 || rows[0].ID != t0.ID {
		t.Fatalf("company_id=0 过滤 total=%d rows=%d，期望唯一命中 %s", total, len(rows), t0.ID)
	}

	seven := uint64(7)
	_, total, err = m.List(ctx, &asynctask.AsyncTaskCond{CompanyID: &seven})
	if err != nil || total != 2 {
		t.Fatalf("company_id=7 过滤 total=%d err=%v，期望 2", total, err)
	}

	_, total, err = m.List(ctx, &asynctask.AsyncTaskCond{SubjectKind: "work", SubjectID: "w-7"})
	if err != nil || total != 1 {
		t.Fatalf("主体过滤 total=%d err=%v，期望 1", total, err)
	}

	_, total, err = m.List(ctx, &asynctask.AsyncTaskCond{UniqKey: "no-such"})
	if err != nil || total != 0 {
		t.Fatalf("uniq_key 过滤 total=%d err=%v，期望 0", total, err)
	}
}

// TestC4GetLatestBySubject 按主体取最近一条：带类型走索引顺序，不带类型取主体内最新。
func TestC4GetLatestBySubject(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	first := submitDim(t, m, 1, "work", "w-latest", "gen")
	time.Sleep(2 * time.Millisecond)
	second := submitDim(t, m, 1, "work", "w-latest", "gen")
	time.Sleep(2 * time.Millisecond)
	other := submitDim(t, m, 1, "work", "w-latest", "export")

	got, err := m.GetLatestBySubject(ctx, "work", "w-latest", "gen")
	if err != nil {
		t.Fatalf("GetLatestBySubject(gen): %v", err)
	}
	if got.ID != second.ID {
		t.Fatalf("gen 最近一条 = %s，期望 %s（first=%s）", got.ID, second.ID, first.ID)
	}

	got, err = m.GetLatestBySubject(ctx, "work", "w-latest", "")
	if err != nil {
		t.Fatalf("GetLatestBySubject(any): %v", err)
	}
	if got.ID != other.ID {
		t.Fatalf("不带类型最近一条 = %s，期望 %s", got.ID, other.ID)
	}

	if _, err := m.GetLatestBySubject(ctx, "work", "nope", ""); !errors.Is(err, asynctask.ErrAsyncTaskNotFound) {
		t.Fatalf("不存在主体 err = %v，期望 ErrAsyncTaskNotFound", err)
	}
}

// TestC4ListActiveOnlyAndKeyset ActiveOnly 与 keyset 分页（created_at desc 游标）。
func TestC4ListActiveOnlyAndKeyset(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	var all []*asynctask.AsyncTask
	for i := 0; i < 5; i++ {
		all = append(all, submitDim(t, m, 9, "work", "w-page", "gen"))
		time.Sleep(2 * time.Millisecond)
	}
	// 把中间一条置 succeeded：ActiveOnly 应排除它。
	forceStatus(t, m, all[2].ID, asynctask.StatusSucceeded)

	rows, total, err := m.List(ctx, &asynctask.AsyncTaskCond{CompanyID: uint64Ptr(9), ActiveOnly: true})
	if err != nil {
		t.Fatalf("List(ActiveOnly): %v", err)
	}
	if total != 4 || len(rows) != 4 {
		t.Fatalf("ActiveOnly total=%d rows=%d，期望 4", total, len(rows))
	}
	for _, r := range rows {
		if r.ID == all[2].ID {
			t.Fatalf("ActiveOnly 不应包含终态任务 %s", all[2].ID)
		}
	}

	// keyset：第一页 2 条（created_at desc），用最后一条做游标取第二页。
	page1, _, err := m.List(ctx, &asynctask.AsyncTaskCond{CompanyID: uint64Ptr(9), PageSize: 2})
	if err != nil || len(page1) != 2 {
		t.Fatalf("keyset page1 rows=%d err=%v", len(page1), err)
	}
	if page1[0].ID != all[4].ID || page1[1].ID != all[3].ID {
		t.Fatalf("page1 顺序错：%s,%s（期望 %s,%s）", page1[0].ID, page1[1].ID, all[4].ID, all[3].ID)
	}
	cursor := page1[len(page1)-1]
	page2, total2, err := m.List(ctx, &asynctask.AsyncTaskCond{
		CompanyID:      uint64Ptr(9),
		PageSize:       2,
		AfterCreatedAt: &cursor.CreatedAt,
		AfterID:        cursor.ID,
	})
	if err != nil {
		t.Fatalf("keyset page2: %v", err)
	}
	if total2 != 3 || len(page2) != 2 {
		t.Fatalf("page2 total=%d rows=%d，期望 3/2", total2, len(page2))
	}
	if page2[0].ID != all[2].ID || page2[1].ID != all[1].ID {
		t.Fatalf("page2 顺序错：%s,%s（期望 %s,%s）", page2[0].ID, page2[1].ID, all[2].ID, all[1].ID)
	}
}

// TestC4StatsCond 条件统计：租户过滤 + 状态分布。
func TestC4StatsCond(t *testing.T) {
	m := newTestManager(t)
	ctx := context.Background()

	submitDim(t, m, 3, "work", "a", "gen")
	done := submitDim(t, m, 3, "work", "b", "gen")
	submitDim(t, m, 4, "work", "c", "gen") // 另一个租户，不应计入
	forceStatus(t, m, done.ID, asynctask.StatusSucceeded)

	st, err := m.StatsCond(ctx, &asynctask.AsyncTaskCond{CompanyID: uint64Ptr(3)})
	if err != nil {
		t.Fatalf("StatsCond: %v", err)
	}
	if st.Total != 2 || st.Pending != 1 || st.Succeeded != 1 {
		t.Fatalf("StatsCond = %+v，期望 total2 pending1 succeeded1", st)
	}

	all, err := m.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if all.Total != 3 {
		t.Fatalf("Stats 全表 total=%d，期望 3", all.Total)
	}
}

// uint64Ptr 返回 uint64 指针（AsyncTaskCond.CompanyID 用指针区分「不过滤」与租户 0）。
func uint64Ptr(v uint64) *uint64 { return &v }
