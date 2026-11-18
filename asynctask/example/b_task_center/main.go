// Command b_task_center 演示「形态 B：完整任务中心」——既要执行，也要管理面
// （查询 / 取消 / 重放 / 统计 / 类型启停）。
//
// 关键点：
//   - manager.New 默认 AutoMigrate 建三张表；必须显式 manager.Start(ctx) 起维护循环，
//     否则「次数用尽 + 租约过期」的卡死任务无人回收、终态记录也不会按 Retention 清理；
//   - worker 复用 manager.Source() 的同一个 DB Source，两处 LeaseTTL 必须一致；
//   - 结束用 Close / Shutdown 释放连接与在途任务。
//
// 运行（sqlite 内存库，无需外部依赖）：
//
//	go run ./asynctask/example/b_task_center
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/manager"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// InvoicePayload 发票开具载荷示例。
type InvoicePayload struct {
	OrderNo string `json:"order_no"`
	Amount  int64  `json:"amount"`
}

const taskType = "invoice:issue"

// InvoiceHandler 实现 worker.Handler：唯一必须实现 Execute。
// 真实业务里应保证幂等（用 task.ID + Generation + Attempt 做幂等键）。
type InvoiceHandler struct{}

func (InvoiceHandler) Execute(ctx context.Context, t *asynctask.AsyncTask) error {
	var p InvoicePayload
	if err := t.Bind(&p); err != nil {
		return worker.ErrNoRetry // 载荷坏了不重试
	}
	// 业务须响应 ctx.Done()：超时/取消通过取消 ctx 表达，忽略它会跑完。
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}
	fmt.Printf("[handler] issue invoice order=%s amount=%d (gen=%d attempt=%d)\n",
		p.OrderNo, p.Amount, t.Generation, t.Attempt)
	return nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := gorm.Open(sqlite.Open("file:example_b?mode=memory&cache=shared"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}

	// 任务中心：默认 AutoMigrate 建表。LeaseTTL 与下方 worker 保持一致。
	center, err := manager.New(&manager.Config{LeaseTTL: 60 * time.Second}, db)
	if err != nil {
		log.Fatalf("manager.New: %v", err)
	}
	defer center.Close()

	// 必须显式启动维护循环（回收卡死任务 + 按 Retention 清理终态记录）。
	if err := center.Start(ctx); err != nil {
		log.Fatalf("manager.Start: %v", err)
	}

	// 执行运行时：复用 center.Source()。
	w := worker.New(center.Source(), &worker.WorkerConfig{
		Concurrency: 5,
		LeaseTTL:    60 * time.Second, // 必须与 manager.Config.LeaseTTL 一致
	})
	if err := w.Register(taskType, InvoiceHandler{}); err != nil {
		log.Fatalf("register: %v", err)
	}
	if err := w.Start(ctx); err != nil {
		log.Fatalf("worker.Start: %v", err)
	}
	defer w.Shutdown(context.Background())

	// 提交：UniqKey 命中时 submitted=false，返回既有任务（幂等投递）。
	payload, _ := json.Marshal(InvoicePayload{OrderNo: "ORD-1001", Amount: 9900})
	task, submitted, err := center.Submit(ctx, &asynctask.AsyncTaskSpec{
		Type:    taskType,
		Payload: payload,
		BizType: "order",
		BizID:   "ORD-1001",
		UniqKey: "invoice:issue:ORD-1001",
	})
	if err != nil {
		log.Fatalf("submit: %v", err)
	}
	fmt.Printf("[submit] id=%s submitted=%v\n", task.ID, submitted)

	// 等执行完成。
	time.Sleep(1500 * time.Millisecond)

	// 查询任务 + 每次尝试记录。
	got, runs, err := center.GetRuns(ctx, task.ID)
	if err != nil {
		log.Fatalf("GetRuns: %v", err)
	}
	fmt.Printf("[query] status=%s attempts=%d runs=%d\n", got.Status, got.Attempt, len(runs))

	// 统计。
	stats, err := center.Stats(ctx)
	if err != nil {
		log.Fatalf("Stats: %v", err)
	}
	fmt.Printf("[stats] total=%d pending=%d succeeded=%d failed=%d\n",
		stats.Total, stats.Pending, stats.Succeeded, stats.Failed)

	// 列表查询（按类型过滤）。
	list, total, err := center.List(ctx, &asynctask.AsyncTaskCond{Type: taskType, PageSize: 10})
	if err != nil {
		log.Fatalf("List: %v", err)
	}
	fmt.Printf("[list] total=%d rows=%d\n", total, len(list))

	fmt.Println("[main] done")
}
