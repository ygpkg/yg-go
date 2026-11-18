// Command a_pure_async 演示「形态 A：简单纯异步」——把「投递」与「执行」两侧彻底拆开，
// 各取所需、互不拖累：
//
//   - 提交侧只依赖 manager：New → Submit → Close，不构造 Worker（只提交不执行是合法用法）；
//   - 执行侧只依赖 source/db + worker：不构造 Manager，也不引任何管理面。
//
// 本示例为了能独立跑通，把两侧放在同一个进程里顺序演示；实际使用时它们通常分属两个进程/服务。
// 建表由 manager.New 默认 AutoMigrate 负责；执行侧 source/db.New 不建表。
//
// 运行（demo_db 用 sqlite 内存库，无需外部依赖）：
//
//	go run ./asynctask/example/a_pure_async
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/manager"
	sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// EmailPayload 是任务载荷示例。
type EmailPayload struct {
	To      string `json:"to"`
	Subject string `json:"subject"`
}

const taskType = "email:send"

// openDB 打开一个 sqlite 内存库做 demo（生产用你的写主库 *gorm.DB）。
// 用唯一的 file: 名 + cache=shared 保证内存库在同一连接池内可见。
func openDB() *gorm.DB {
	db, err := gorm.Open(sqlite.Open("file:example_a?mode=memory&cache=shared"), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		log.Fatalf("open sqlite: %v", err)
	}
	return db
}

// submitSide 是「提交侧」：只需要 manager，不引 worker。
func submitSide(ctx context.Context, db *gorm.DB, p EmailPayload) (*asynctask.AsyncTask, error) {
	center, err := manager.New(&manager.Config{}, db) // 默认 AutoMigrate 建表
	if err != nil {
		return nil, err
	}
	defer center.Close()

	payload, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}

	// Submit 三值语义：task / submitted / err。
	//   err == nil          —— 成功；
	//   err == ErrNotify    —— 已落库但唤醒通知没发出（勿重试，兜底轮询会接手）；
	//   submitted           —— 是否新建（UniqKey 命中时 false，返回既有任务）。
	task, submitted, err := center.Submit(ctx, &asynctask.AsyncTaskSpec{
		Type:    taskType,
		Payload: payload,
	})
	if err != nil && !errors.Is(err, asynctask.ErrNotify) {
		return nil, err
	}
	fmt.Printf("[submit] id=%s submitted=%v notifyErr=%v\n", task.ID, submitted, err)
	return task, nil
}

// executeSide 是「执行侧」：只需要 source/db + worker，不构造 Manager。
func executeSide(ctx context.Context, db *gorm.DB) (*worker.Worker, error) {
	src, err := sourcedb.New(sourcedb.Config{DB: db, LeaseTTL: 60 * time.Second})
	if err != nil {
		return nil, err
	}

	// WorkerConfig.LeaseTTL 必须与 sourcedb.Config.LeaseTTL 一致：Worker 用它决定续租周期，
	// 小了会被误判失权，大了会让崩溃后的接管变慢。
	w := worker.New(src, &worker.WorkerConfig{Concurrency: 10, LeaseTTL: 60 * time.Second})

	err = w.Register(taskType, worker.HandlerFunc(func(ctx context.Context, t *asynctask.AsyncTask) error {
		var p EmailPayload
		if err := t.Bind(&p); err != nil {
			return worker.ErrNoRetry // payload 反序列化失败：不重试，直接 failed
		}
		return sendEmail(ctx, p)
	}))
	if err != nil {
		return nil, err
	}

	if err := w.Start(ctx); err != nil { // 同步非阻塞，不需要 go 关键字
		return nil, err
	}
	return w, nil
}

func sendEmail(_ context.Context, p EmailPayload) error {
	fmt.Printf("[execute] send email to=%s subject=%s\n", p.To, p.Subject)
	return nil
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db := openDB()

	// 1) 提交侧：投递一个任务。
	if _, err := submitSide(ctx, db, EmailPayload{To: "a@example.com", Subject: "hello"}); err != nil {
		log.Fatalf("submit: %v", err)
	}

	// 2) 执行侧：起 worker，轮询认领并执行。
	w, err := executeSide(ctx, db)
	if err != nil {
		log.Fatalf("execute side: %v", err)
	}
	defer w.Shutdown(context.Background())

	// 等待任务被消费（demo 用固定等待，生产用事件/信号）。
	time.Sleep(2 * time.Second)
	fmt.Println("[main] done")
}
