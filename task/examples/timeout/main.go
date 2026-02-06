package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/task"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// TimeoutPayload 超时任务的参数结构
type TimeoutPayload struct {
	Message      string `json:"message"`
	Duration     int    `json:"duration"`      // 任务执行时长（秒）
	CheckContext bool   `json:"check_context"` // 是否检查上下文取消
}

// TimeoutTaskExecutor 演示超时处理的任务执行器
type TimeoutTaskExecutor struct {
	task.BaseExecutor
	payload TimeoutPayload
}

// Prepare 初始化执行器
func (e *TimeoutTaskExecutor) Prepare(ctx context.Context, taskEntity *task.TaskEntity) error {
	if err := e.BaseExecutor.Prepare(ctx, taskEntity); err != nil {
		return err
	}

	// 解析任务参数
	if err := json.Unmarshal([]byte(taskEntity.Payload), &e.payload); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("\n════════════════════════════════════════\n")
	fmt.Printf("准备执行任务 %d\n", taskEntity.ID)
	fmt.Printf("════════════════════════════════════════\n")
	fmt.Printf("任务执行时长: %d 秒\n", e.payload.Duration)
	fmt.Printf("任务超时时间: %.0f 秒\n", taskEntity.Timeout.Seconds())
	fmt.Printf("检查上下文取消: %v\n", e.payload.CheckContext)
	fmt.Printf("最大重试次数: %d\n", taskEntity.MaxRedo)

	return nil
}

// Execute 执行任务
func (e *TimeoutTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("\n→ 开始执行任务...\n")

	duration := time.Duration(e.payload.Duration) * time.Second

	if e.payload.CheckContext {
		// 模拟长时间运行的任务，定期检查上下文
		return e.executeWithContextCheck(ctx, duration)
	} else {
		// 模拟长时间运行的任务，不检查上下文
		return e.executeWithoutContextCheck(duration)
	}
}

// executeWithContextCheck 执行任务并检查上下文取消
func (e *TimeoutTaskExecutor) executeWithContextCheck(ctx context.Context, duration time.Duration) error {
	fmt.Println("  使用上下文检查模式")

	// 模拟分批处理，每秒检查一次上下文
	deadline := time.Now().Add(duration)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	processed := 0
	total := int(duration.Seconds())

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			// 上下文被取消（超时）
			fmt.Printf("\n✗ 检测到上下文取消: %v\n", ctx.Err())
			fmt.Printf("  已处理: %d/%d 项\n", processed, total)
			return ctx.Err()

		case <-ticker.C:
			processed++
			fmt.Printf("  处理进度: %d/%d\n", processed, total)
		}
	}

	fmt.Printf("\n✓ 任务执行完成 (共处理 %d 项)\n", processed)
	return nil
}

// executeWithoutContextCheck 执行任务但不检查上下文
func (e *TimeoutTaskExecutor) executeWithoutContextCheck(duration time.Duration) error {
	fmt.Println("  使用非上下文检查模式（不推荐）")
	fmt.Printf("  → 睡眠 %d 秒...\n", e.payload.Duration)

	// 直接睡眠，不检查上下文
	time.Sleep(duration)

	fmt.Printf("\n✓ 任务执行完成\n")
	return nil
}

// OnSuccess 成功回调
func (e *TimeoutTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("\n✓ OnSuccess: 任务成功完成\n")
	return nil
}

// OnFailure 失败回调
func (e *TimeoutTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("\n✗ OnFailure: 任务执行失败\n")
	fmt.Printf("  当前重试次数: %d\n", e.Task.Redo)

	if e.Task.CanRetry() {
		fmt.Printf("  → 将进行重试 (剩余 %d 次)\n", e.Task.MaxRedo-e.Task.Redo)
	} else {
		fmt.Printf("  ✗ 已达到最大重试次数，不再重试\n")
	}

	return nil
}

// setupDB 使用 gorm 原生方式创建数据库连接
func setupDB() (*gorm.DB, error) {
	// MySQL DSN 格式: user:pass@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	dsn := "root:root@tcp(localhost:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local"

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		CreateBatchSize: 200,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	return db, nil
}

// setupRedis 使用 go-redis 原生方式创建 Redis 客户端
func setupRedis() (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // 无密码
		DB:       0,  // 默认 DB
	})

	// 测试连接
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect redis: %w", err)
	}

	return client, nil
}

func main() {
	fmt.Println("========================================")
	fmt.Println("Task 包 - 任务超时处理演示")
	fmt.Println("========================================\n")

	// 配置 Worker
	config := &task.TaskConfig{
		WorkerID:          "timeout-worker-001",
		MaxConcurrency:    3,
		Timeout:           10 * time.Minute,
		MaxRedo:           2, // 超时后最多重试 2 次
		RedisKeyPrefix:    "task:timeout:",
		EnableHealthCheck: true,
		HealthCheckPeriod: 30 * time.Second,
	}

	// 创建 Worker
	db, err := setupDB()
	if err != nil {
		fmt.Printf("✗ 数据库连接失败: %v\n", err)
		fmt.Println("\n提示: 请确保 MySQL 服务正在运行")
		os.Exit(1)
	}

	redisClient, err := setupRedis()
	if err != nil {
		fmt.Printf("✗ Redis 连接失败: %v\n", err)
		fmt.Println("\n提示: 请确保 Redis 服务正在运行")
		os.Exit(1)
	}

	worker, err := task.NewWorker(config, db, redisClient)
	if err != nil {
		fmt.Printf("✗ 创建 Worker 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Worker 创建成功")

	// 注册任务执行器
	worker.RegisterExecutor("timeout_task", func() task.TaskExecutor {
		return &TimeoutTaskExecutor{}
	})
	fmt.Println("✓ 已注册执行器: timeout_task\n")

	// 启动 Worker
	ctx := context.Background()
	if err := worker.Start(ctx); err != nil {
		fmt.Printf("✗ 启动 Worker 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Worker 已启动\n")

	defer func() {
		fmt.Println("\n========================================")
		fmt.Println("停止 Worker")
		fmt.Println("========================================")
		if err := worker.Stop(ctx); err != nil {
			fmt.Printf("✗ 停止 Worker 失败: %v\n", err)
		} else {
			fmt.Println("✓ Worker 已停止")
		}
	}()

	// 演示场景 1: 任务会超时（不检查上下文）
	fmt.Println("========================================")
	fmt.Println("场景 1: 任务超时（不检查上下文）")
	fmt.Println("========================================")
	fmt.Println("任务配置: 执行 10 秒，超时时间 5 秒")
	fmt.Println("预期结果: 超时并重试\n")

	payload1 := TimeoutPayload{
		Message:      "这是一个会超时的任务",
		Duration:     10,    // 执行 10 秒
		CheckContext: false, // 不检查上下文
	}

	payloadJSON1, _ := json.Marshal(payload1)

	task1 := &task.TaskEntity{
		TaskType:    "timeout_task",
		SubjectType: "timeout_demo",
		SubjectID:   1,
		Payload:     string(payloadJSON1),
		Timeout:     5 * time.Second, // 5 秒超时
		MaxRedo:     2,
		Priority:    0,
		CompanyID:   1,
		Uin:         1001,
	}
	payload2 := TimeoutPayload{
		Message:      "这是一个正常完成的任务",
		Duration:     3,    // 执行 3 秒
		CheckContext: true, // 检查上下文
	}

	payloadJSON2, _ := json.Marshal(payload2)

	task2 := &task.TaskEntity{
		TaskType:    "timeout_task",
		SubjectType: "timeout_demo",
		SubjectID:   2,
		Payload:     string(payloadJSON2),
		Timeout:     10 * time.Second, // 10 秒超时
		MaxRedo:     2,
		Priority:    0,
		CompanyID:   1,
		Uin:         1001,
	}
	taskEntityList := []*task.TaskEntity{task1, task2}
	if err := worker.CreateTasks(ctx, taskEntityList); err != nil {
		fmt.Printf("✗ 创建任务失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ 任务 1 已创建，ID: %d\n", task1.ID)
	fmt.Printf("✓ 任务 2 已创建，ID: %d\n", task2.ID)

	// 监控任务状态
	fmt.Println("\n========================================")
	fmt.Println("监控任务执行")
	fmt.Println("========================================")

	// 演示场景 1: 任务会超时（不检查上下文）
	fmt.Println("\n场景 1: 任务超时（不检查上下文）")
	fmt.Println("预期结果: 任务会超时并重试，最终失败\n")

	// 等待第一个任务完成
	monitorTask(worker, task1.ID, "场景 1", 30*time.Second)

	// 演示场景 2: 任务正常完成（检查上下文）
	fmt.Println("\n========================================")
	fmt.Println("场景 2: 任务正常完成（检查上下文）")
	fmt.Println("========================================")
	fmt.Println("任务配置: 执行 3 秒，超时时间 10 秒")
	fmt.Println("预期结果: 正常完成\n")

	// 等待第二个任务完成
	monitorTask(worker, task2.ID, "场景 2", 30*time.Second)

	// 总结
	fmt.Println("\n========================================")
	fmt.Println("超时处理要点")
	fmt.Println("========================================")
	fmt.Println("1. 设置合理的超时时间（基于任务预期执行时间）")
	fmt.Println("2. 在长时间运行的任务中定期检查 ctx.Done()")
	fmt.Println("3. 超时的任务会自动标记为 timeout 状态")
	fmt.Println("4. 超时任务会根据 MaxRedo 配置自动重试")
	fmt.Println("5. 检查上下文可以实现优雅的超时退出")
	fmt.Println("6. 不检查上下文会导致任务继续执行直到完成")

	fmt.Println("\n按 Ctrl+C 退出...")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

// monitorTask 监控任务执行
func monitorTask(worker *task.Worker, taskID uint, scenarioName string, timeout time.Duration) {
	ctx := context.Background()
	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	lastStatus := ""

	for {
		select {
		case <-timeoutCh:
			fmt.Printf("\n✗ %s: 监控超时\n", scenarioName)
			return

		case <-ticker.C:
			result, err := worker.GetTask(ctx, taskID)
			if err != nil {
				fmt.Printf("✗ 获取任务状态失败: %v\n", err)
				return
			}

			// 只在状态改变时输出
			if string(result.TaskStatus) != lastStatus {
				fmt.Printf("[监控] 状态: %s | 重试: %d/%d\n",
					result.TaskStatus, result.Redo, result.MaxRedo)
				lastStatus = string(result.TaskStatus)
			}

			// 检查任务是否完成
			if result.IsFinished() {
				fmt.Printf("\n%s 结果:\n", scenarioName)
				fmt.Printf("  最终状态: %s\n", result.TaskStatus)
				fmt.Printf("  重试次数: %d\n", result.Redo)
				fmt.Printf("  执行耗时: %d 秒\n", result.Cost)

				if result.IsSuccess() {
					fmt.Println("  ✓ 任务成功")
				} else {
					fmt.Printf("  ✗ 任务失败: %s\n", result.ErrMsg)
				}

				return
			}
		}
	}
}
