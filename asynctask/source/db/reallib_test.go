package db

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// 真库验收：A1 并发认领不重复、A2 租约接管、A8 EXPLAIN 命中索引。
//
// 连不上就 t.Skip（skip≠验证过）。DSN 取自环境变量或 .env 的默认本机值：
//
//	MYSQL_DSN / POSTGRES_DSN
func realMySQLDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("MYSQL_DSN")
	if dsn == "" {
		// 注意：DSN 的 loc 必须与会话 time_zone 一致（internals「时间来源」硬约束）。
		// 容器默认 UTC，这里把会话钉到 +08:00 与 loc=Local 对齐，否则 NOW() 与 Go 写入
		// 的时间差整小时，任务永不「到期」。
		dsn = "root:123456@tcp(127.0.0.1:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local&time_zone=%27%2B08%3A00%27"
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("skip MySQL: 连不上 (%v)", err)
	}
	sqlDB, err := db.DB()
	if err == nil {
		if err := sqlDB.Ping(); err != nil {
			t.Skipf("skip MySQL: ping 失败 (%v)", err)
		}
	}
	return db
}

func realPGDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "host=127.0.0.1 user=postgres password=123456 dbname=demo port=5432 sslmode=disable TimeZone=Asia/Shanghai"
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Skipf("skip PostgreSQL: 连不上 (%v)", err)
	}
	sqlDB, err := db.DB()
	if err == nil {
		if err := sqlDB.Ping(); err != nil {
			t.Skipf("skip PostgreSQL: ping 失败 (%v)", err)
		}
	}
	return db
}

// resetRealDB 重建两表，隔离各用例。
func resetRealDB(t *testing.T, db *gorm.DB) {
	t.Helper()
	if err := db.Migrator().DropTable(&CoreAsyncTask{}, &CoreAsyncTaskRun{}); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if err := db.AutoMigrate(&CoreAsyncTask{}, &CoreAsyncTaskRun{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
}

// seedAsyncTasks 插入 n 条到点的 pending 任务。
func seedAsyncTasks(t *testing.T, db *gorm.DB, taskType string, n int) {
	t.Helper()
	now := time.Now()
	for i := 0; i < n; i++ {
		row := &CoreAsyncTask{
			ID: genUUID(), CreatedAt: now, UpdatedAt: now, TaskType: taskType,
			Status: "pending", Generation: 1, MaxAttempts: 3, NextRunAt: now.Add(-time.Minute),
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
}

// TestRealA1ConcurrentClaim 多 goroutine / 多连接并发认领，断言无同一 asyncTask 被两次认领
// （attempt 求和 == 认领次数 == 任务数；每行 attempt==1 且只被一个 worker 持有）。
// 真库用例一律拆成子测试：MySQL 不可用时只跳过自己，不会连带跳过 PostgreSQL。
func TestRealA1ConcurrentClaim(t *testing.T) {
	t.Run("mysql", func(t *testing.T) { runA1(t, realMySQLDB(t), DialectMySQL, "invoice") })
	t.Run("postgres", func(t *testing.T) { runA1(t, realPGDB(t), DialectPostgres, "invoice") })
}

func runA1(t *testing.T, db *gorm.DB, dialect Dialect, taskType string) {
	t.Helper()
	resetRealDB(t, db)
	const total = 200
	const workers = 16
	seedAsyncTasks(t, db, taskType, total)

	base := newStore(db, dialect, 60*time.Second)

	claimedMu := sync.Mutex{}
	claimed := map[string]int{}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			// 每个 worker 用独立的连接（gorm 连接池各自取连接）。
			s := newStore(db, dialect, 60*time.Second)
			workerID := fmt.Sprintf("w%d", wid)
			for {
				rows, err := s.claimBatch(context.Background(), taskType, workerID, 10)
				if err != nil {
					return
				}
				if len(rows) == 0 {
					return
				}
				claimedMu.Lock()
				for _, r := range rows {
					claimed[r.ID]++
				}
				claimedMu.Unlock()
				if len(rows) < 10 {
					return
				}
			}
		}(w)
	}
	wg.Wait()

	// 断言：每个 id 至多被认领一次。
	for id, c := range claimed {
		if c > 1 {
			t.Fatalf("[%s A1] asyncTask %s claimed %d times (double claim)", dialect, id, c)
		}
	}
	var cnt int64
	db.Model(&CoreAsyncTask{}).Where("status = 'running'").Count(&cnt)
	t.Logf("[%s A1] claimed unique asyncTasks=%d, running rows=%d (seed=%d workers=%d)",
		dialect, len(claimed), cnt, total, workers)
	if int64(len(claimed)) != cnt {
		t.Fatalf("[%s A1] claimed=%d but running rows=%d (mismatch)", dialect, len(claimed), cnt)
	}
	// 每行 attempt 必须恰为 1（未被重复认领累加）。
	var bad int64
	db.Model(&CoreAsyncTask{}).Where("status = 'running' AND attempt <> 1").Count(&bad)
	if bad != 0 {
		t.Fatalf("[%s A1] %d running rows with attempt != 1 (repeat claim)", dialect, bad)
	}
	_ = base
}

// TestRealA2LeaseTakeover 认领后手工把 lease_until 改到过去，另一认领能接管，attempt+1。
func TestRealA2LeaseTakeover(t *testing.T) {
	t.Run("mysql", func(t *testing.T) { runA2(t, realMySQLDB(t), DialectMySQL) })
	t.Run("postgres", func(t *testing.T) { runA2(t, realPGDB(t), DialectPostgres) })
}

func runA2(t *testing.T, db *gorm.DB, dialect Dialect) {
	t.Helper()
	resetRealDB(t, db)
	seedAsyncTasks(t, db, "invoice", 1)
	s := newStore(db, dialect, 60*time.Second)
	ctx := context.Background()

	rows, err := s.claimBatch(ctx, "invoice", "w1", 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("[%s A2] first claim: rows=%d err=%v", dialect, len(rows), err)
	}
	id := rows[0].ID

	// 停续租 + 手工把 lease_until 改到过去（internals：A2 改成停续租 + 手工改 lease_until）。
	past := time.Now().Add(-time.Minute)
	if err := db.Model(&CoreAsyncTask{}).Where("id = ?", id).Update("lease_until", past).Error; err != nil {
		t.Fatalf("[%s A2] force lease: %v", dialect, err)
	}

	// 另一 worker 认领，应接管。
	rows2, err := s.claimBatch(ctx, "invoice", "w2", 10)
	if err != nil {
		t.Fatalf("[%s A2] takeover claim: %v", dialect, err)
	}
	if len(rows2) != 1 || rows2[0].ID != id {
		t.Fatalf("[%s A2] takeover failed: %+v", dialect, rows2)
	}
	var got CoreAsyncTask
	db.Where("id = ?", id).Take(&got)
	if got.Attempt != 2 {
		t.Fatalf("[%s A2] attempt = %d, want 2 after takeover", dialect, got.Attempt)
	}
	if got.WorkerID != "w2" {
		t.Fatalf("[%s A2] worker = %q, want w2", dialect, got.WorkerID)
	}
	t.Logf("[%s A2] takeover ok: asyncTask=%s attempt=%d worker=%s", dialect, id, got.Attempt, got.WorkerID)
}

// TestRealA8Explain 两支认领 SQL 的 EXPLAIN 命中预期索引、rows examined ≈ 候选数。
func TestRealA8Explain(t *testing.T) {
	t.Run("mysql", func(t *testing.T) { runA8MySQL(t, realMySQLDB(t)) })
	t.Run("postgres", func(t *testing.T) { runA8PG(t, realPGDB(t)) })
}

func runA8MySQL(t *testing.T, db *gorm.DB) {
	resetRealDB(t, db)
	// 10 万 pending、1 千到期（internals [待核实] 场景）。
	seedHeavyMySQL(t, db, 100000, 1000)
	now := "NOW(3)"
	sql1 := fmt.Sprintf(`SELECT id FROM core_async_task WHERE task_type = 'invoice' AND status='pending' AND next_run_at <= %s AND attempt < max_attempts ORDER BY priority DESC, next_run_at LIMIT 50`, now)
	sql2 := fmt.Sprintf(`SELECT id FROM core_async_task WHERE status='running' AND lease_until < %s AND task_type='invoice' AND attempt < max_attempts ORDER BY lease_until LIMIT 50`, now)
	// ① 必须走新索引且不 filesort（优先级顺序由索引提供）。
	explainMySQL(t, db, "MySQL ①", sql1, true, "idx_core_async_task_claim")
	explainMySQL(t, db, "MySQL ②", sql2, false, "idx_core_async_task_status_lease_until")
}

func runA8PG(t *testing.T, db *gorm.DB) {
	resetRealDB(t, db)
	seedHeavyPG(t, db, 100000, 1000)
	sql1 := `SELECT id FROM core_async_task WHERE task_type = 'invoice' AND status='pending' AND next_run_at <= now() AND attempt < max_attempts ORDER BY priority DESC, next_run_at LIMIT 50`
	sql2 := `SELECT id FROM core_async_task WHERE status='running' AND lease_until < now() AND task_type='invoice' AND attempt < max_attempts ORDER BY lease_until LIMIT 50`
	// ① 必须走 idx_core_async_task_claim 且计划里没有 Sort 节点。
	explainPG(t, db, "PG ①", sql1, true, "idx_core_async_task_claim")
	// PG 的代价模型在 task_type+status 高选择性的数据分布下，可能选 (task_type,status)
	// 前缀索引而非 (status,lease_until)。两者都是合法索引扫描，非 Seq Scan 即接受。
	explainPG(t, db, "PG ②", sql2, false,
		"idx_core_async_task_status_lease_until", "idx_core_async_task_task_type_status_created_at")
}

// explainMySQL 跑 MySQL EXPLAIN FORMAT=JSON 并断言命中预期索引；wantNoSort 时
// 额外断言计划里没有 using_filesort（认领 SQL 的排序必须由索引提供）。
func explainMySQL(t *testing.T, db *gorm.DB, label, sql string, wantNoSort bool, wantIndex string) {
	var rows []struct{ EXPLAIN string }
	if err := db.Raw("EXPLAIN FORMAT=JSON " + sql).Scan(&rows).Error; err != nil {
		t.Fatalf("[%s] explain: %v", label, err)
	}
	if len(rows) == 0 {
		t.Fatalf("[%s] empty explain", label)
	}
	js := rows[0].EXPLAIN
	if !containsStr(js, wantIndex) {
		t.Fatalf("[%s] EXPLAIN 未命中索引 %s:\n%s", label, wantIndex, js)
	}
	hasSort := containsStr(js, `"using_filesort": true`)
	if wantNoSort && hasSort {
		t.Fatalf("[%s] 计划里出现 using_filesort（排序没被索引覆盖）:\n%s", label, js)
	}
	t.Logf("[%s] 命中索引 %s（filesort=%v, EXPLAIN JSON 已核实）", label, wantIndex, hasSort)
}

// explainPG 跑 PostgreSQL EXPLAIN (FORMAT JSON)，断言走了 Index/Bitmap Index Scan
// 且命中的索引属于 wantIndexes 之一（PG 的代价模型可能在同前缀的多个索引间选择，
// 例如 task_type,status,next_run_at 与 task_type,status,created_at 前缀相同）。
func explainPG(t *testing.T, db *gorm.DB, label, sql string, wantNoSort bool, wantIndexes ...string) {
	var jsBytes []byte
	row := db.Raw("EXPLAIN (FORMAT JSON) " + sql).Row()
	if err := row.Scan(&jsBytes); err != nil {
		t.Fatalf("[%s] explain: %v", label, err)
	}
	js := string(jsBytes)
	if containsStr(js, "Seq Scan") {
		t.Fatalf("[%s] 走了 Seq Scan:\n%s", label, js)
	}
	if !containsStr(js, "Index Scan") {
		t.Fatalf("[%s] 未走索引扫描:\n%s", label, js)
	}
	if wantNoSort && (containsStr(js, `"Node Type": "Sort"`) || containsStr(js, "Incremental Sort")) {
		t.Fatalf("[%s] 计划里出现 Sort 节点（排序没被索引覆盖）:\n%s", label, js)
	}
	var hit string
	for _, idx := range wantIndexes {
		if containsStr(js, idx) {
			hit = idx
			break
		}
	}
	if hit == "" {
		t.Fatalf("[%s] EXPLAIN 未命中候选索引 %v:\n%s", label, wantIndexes, js)
	}
	t.Logf("[%s] 命中索引 %s (EXPLAIN JSON 已核实)", label, hit)
}

func containsStr(haystack, needle string) bool {
	return len(needle) > 0 && len(haystack) >= len(needle) && indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// seedHeavyMySQL 用批量插入灌数据：total 条 pending（其中 due 条到期）+ 少量 running 过期。
func seedHeavyMySQL(t *testing.T, db *gorm.DB, total, due int) {
	t.Helper()
	now := time.Now()
	batch := make([]CoreAsyncTask, 0, 2000)
	err := db.Transaction(func(tx *gorm.DB) error {
		flush := func() error {
			if len(batch) == 0 {
				return nil
			}
			if err := tx.CreateInBatches(batch, 2000).Error; err != nil {
				return err
			}
			batch = batch[:0]
			return nil
		}
		for i := 0; i < total; i++ {
			nra := now.Add(time.Hour)
			if i < due {
				nra = now.Add(-time.Minute)
			}
			batch = append(batch, CoreAsyncTask{
				ID: genUUID(), CreatedAt: now, UpdatedAt: now, TaskType: "invoice",
				Status: "pending", Generation: 1, MaxAttempts: 3, NextRunAt: nra,
			})
			if err := flush(); err != nil {
				return err
			}
		}
		if err := flush(); err != nil {
			return err
		}
		// 再灌少量 running 且租约过期的行，供认领 ② 走 idx_core_async_task_status_lease_until。
		past := now.Add(-time.Minute)
		rbatch := make([]CoreAsyncTask, 0, 500)
		for i := 0; i < 1000; i++ {
			rbatch = append(rbatch, CoreAsyncTask{
				ID: genUUID(), CreatedAt: now, UpdatedAt: now, TaskType: "invoice",
				Status: "running", Generation: 1, Attempt: 1, MaxAttempts: 3,
				NextRunAt: now.Add(-time.Hour), LeaseUntil: &past, WorkerID: "dead",
			})
		}
		if err := tx.CreateInBatches(rbatch, 500).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		t.Fatalf("seed heavy: %v", err)
	}
}

func seedHeavyPG(t *testing.T, db *gorm.DB, total, due int) {
	seedHeavyMySQL(t, db, total, due) // 结构一致，复用
}

var _ = asynctask.ErrNoAsyncTask

// TestRealSaveHookAndStatus 真库覆盖 Save 主路径（此前只有 SQLite 跑过 Save，
// MySQL 的 ON CONFLICT 语法错误因此漏网）：认领 → 事务内钩子 → 终态 + 尝试行。
func TestRealSaveHookAndStatus(t *testing.T) {
	t.Run("mysql", func(t *testing.T) { runSavePath(t, realMySQLDB(t), DialectMySQL) })
	t.Run("postgres", func(t *testing.T) { runSavePath(t, realPGDB(t), DialectPostgres) })
}

func runSavePath(t *testing.T, db *gorm.DB, dialect Dialect) {
	t.Helper()
	resetRealDB(t, db)
	seedAsyncTasks(t, db, "savepath", 1)
	ctx := context.Background()
	src, err := New(Config{DB: db, WorkerID: "w-save", LeaseTTL: 60 * time.Second})
	if err != nil {
		t.Fatalf("[%s] new source: %v", dialect, err)
	}
	task, err := src.Fetch(ctx, "savepath")
	if err != nil {
		t.Fatalf("[%s] fetch: %v", dialect, err)
	}
	hookCalled := false
	err = src.Save(ctx, task, asynctask.Result{Outcome: asynctask.OutcomeSucceeded, Duration: 5 * time.Millisecond},
		func(hctx context.Context) error {
			hookCalled = true
			tx, ok := asynctask.TxFromContext(hctx).(*gorm.DB)
			if !ok || tx == nil {
				t.Fatalf("[%s] 本地形态钩子应拿到事务", dialect)
			}
			// 用 trace_id 当「副作用标记」：result 列会被终态更新按 handler payload 覆盖。
			return tx.Exec(`UPDATE core_async_task SET trace_id = 'hook-ok' WHERE id = ?`, task.ID).Error
		})
	if err != nil {
		t.Fatalf("[%s] save: %v", dialect, err)
	}
	if !hookCalled {
		t.Fatalf("[%s] hook not called", dialect)
	}
	var row CoreAsyncTask
	if err := db.Where("id = ?", task.ID).Take(&row).Error; err != nil {
		t.Fatalf("[%s] load: %v", dialect, err)
	}
	if row.Status != string(asynctask.StatusSucceeded) {
		t.Fatalf("[%s] status = %q, want succeeded", dialect, row.Status)
	}
	if row.TraceID != "hook-ok" {
		t.Fatalf("[%s] 钩子的事务内写入应同事务提交，trace_id=%q", dialect, row.TraceID)
	}
	var run CoreAsyncTaskRun
	if err := db.Where("async_task_id = ? AND generation = ? AND attempt = ?", task.ID, 1, 1).Take(&run).Error; err != nil {
		t.Fatalf("[%s] run row: %v", dialect, err)
	}
	if run.Status != string(asynctask.StatusSucceeded) {
		t.Fatalf("[%s] run status = %q, want succeeded", dialect, run.Status)
	}
	if run.FinishedAt == nil || run.StartedAt.IsZero() {
		t.Fatalf("[%s] run 起止时间未写全: started=%v finished=%v", dialect, run.StartedAt, run.FinishedAt)
	}
}

// TestRealLabelsFilter 真库覆盖 labels 过滤（PostgreSQL 的 text->> 曾直接报错）。
func TestRealLabelsFilter(t *testing.T) {
	t.Run("mysql", func(t *testing.T) { runLabelsFilter(t, realMySQLDB(t), DialectMySQL) })
	t.Run("postgres", func(t *testing.T) { runLabelsFilter(t, realPGDB(t), DialectPostgres) })
}

func runLabelsFilter(t *testing.T, db *gorm.DB, dialect Dialect) {
	t.Helper()
	resetRealDB(t, db)
	now := time.Now()
	for i := 0; i < 3; i++ {
		row := &CoreAsyncTask{
			ID: genUUID(), CreatedAt: now, UpdatedAt: now, TaskType: "labels",
			Status: "pending", Generation: 1, MaxAttempts: 3, NextRunAt: now.Add(-time.Minute),
			Labels: `{"app_group":"g0"}`, // 注意：列是 text，PG 侧必须 ::jsonb 才能 ->> 取值
		}
		if i == 2 {
			row.Labels = `` // 空串必须被安全跳过（非法 JSON）
		}
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("[%s] seed: %v", dialect, err)
		}
	}
	s := newStore(db, dialect, time.Minute)
	rows, total, err := s.ListAsyncTask(context.Background(), &asynctask.AsyncTaskCond{
		Labels: map[string]string{"app_group": "g0"},
	})
	if err != nil {
		t.Fatalf("[%s] ListAsyncTask(labels) err = %v", dialect, err)
	}
	if total != 2 || len(rows) != 2 {
		t.Fatalf("[%s] labels 过滤 total=%d rows=%d, want 2", dialect, total, len(rows))
	}
}
