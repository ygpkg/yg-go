package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/task"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// RetryPayload 重试任务的参数结构
type RetryPayload struct {
	Message    string `json:"message"`
	FailTimes  int    `json:"fail_times"`  // 前几次失败
	FailReason string `json:"fail_reason"` // 失败原因
}

// RetryTaskExecutor 演示重试机制的任务执行器
type RetryTaskExecutor struct {
	task.BaseExecutor
	payload        RetryPayload
	attemptCount   *int32 // 记录执行次数（跨实例共享需要外部传入）
	currentAttempt int32
}

// Prepare 初始化执行器
func (e *RetryTaskExecutor) Prepare(ctx context.Context, taskEntity *task.TaskEntity) error {
	if err := e.BaseExecutor.Prepare(ctx, taskEntity); err != nil {
		return err
	}

	// 解析任务参数
	if err := json.Unmarshal([]byte(taskEntity.Payload), &e.payload); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// 增加尝试计数
	e.currentAttempt = atomic.AddInt32(e.attemptCount, 1)

	fmt.Printf("\n════════════════════════════════════════\n")
	fmt.Printf("准备执行任务 (第 %d 次尝试)\n", e.currentAttempt)
	fmt.Printf("════════════════════════════════════════\n")
	fmt.Printf("任务 ID: %d\n", taskEntity.ID)
	fmt.Printf("当前重试次数: %d\n", taskEntity.Redo)
	fmt.Printf("最大重试次数: %d\n", taskEntity.MaxRedo)
	fmt.Printf("配置的失败次数: %d\n", e.payload.FailTimes)

	return nil
}

// Execute 执行任务
func (e *RetryTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("\n→ 开始执行任务...\n")

	// 模拟任务处理
	time.Sleep(1 * time.Second)

	// 根据配置决定是否失败
	if int(e.currentAttempt) <= e.payload.FailTimes {
		errMsg := fmt.Sprintf("%s (尝试 %d/%d)",
			e.payload.FailReason,
			e.currentAttempt,
			e.payload.FailTimes)
		fmt.Printf("✗ 任务执行失败: %s\n", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	fmt.Printf("✓ 任务执行成功 (尝试 %d 次后成功)\n", e.currentAttempt)
	return nil
}

// OnSuccess 成功回调
func (e *RetryTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("\n✓ OnSuccess: 任务最终成功\n")
	fmt.Printf("  总尝试次数: %d\n", e.currentAttempt)
	fmt.Printf("  重试次数: %d\n", e.Task.Redo)
	return nil
}

// OnFailure 失败回调
func (e *RetryTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
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
	dsn := "root:root@tcp(localhost:3306)/task_demo?charset=utf8mb4&parseTime=True&loc=Local"

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
	fmt.Println("Task 包 - 任务重试机制演示")
	fmt.Println("========================================\n")

	// 配置 Worker
	config := &task.TaskConfig{
		WorkerID:          "retry-worker-001",
		MaxConcurrency:    3,
		Timeout:           10 * time.Minute,
		MaxRedo:           3, // 最多重试 3 次
		RedisKeyPrefix:    "task:retry:",
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

	// 用于记录执行次数的计数器
	var attemptCount int32

	// 注册任务执行器
	worker.RegisterExecutor("retry_task", func() task.TaskExecutor {
		return &RetryTaskExecutor{
			attemptCount: &attemptCount,
		}
	})
	fmt.Println("✓ 已注册执行器: retry_task\n")

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

	// 创建任务：前 2 次失败，第 3 次成功
	fmt.Println("========================================")
	fmt.Println("创建测试任务")
	fmt.Println("========================================")
	fmt.Println("任务配置: 前 2 次失败，第 3 次成功")
	fmt.Println()

	payload := RetryPayload{
		Message:    "这是一个会重试的任务",
		FailTimes:  2, // 前 2 次失败
		FailReason: "模拟的临时错误",
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("✗ 序列化参数失败: %v\n", err)
		os.Exit(1)
	}

	taskEntity := &task.TaskEntity{
		TaskType:    "retry_task",
		SubjectType: "retry_demo",
		SubjectID:   1,
		Payload:     string(payloadJSON),
		Timeout:     5 * time.Minute,
		MaxRedo:     3, // 最多重试 3 次
		Priority:    0,
		CompanyID:   1,
		Uin:         1001,
	}
	taskEntityList := []*task.TaskEntity{taskEntity}

	if err := worker.CreateTasks(ctx, taskEntityList); err != nil {
		fmt.Printf("✗ 创建任务失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ 任务已创建，ID: %d\n", taskEntity.ID)

	// 监控任务状态
	fmt.Println("\n========================================")
	fmt.Println("监控任务执行")
	fmt.Println("========================================")

	taskID := taskEntity.ID
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	lastStatus := ""
	lastRedo := -1

	for {
		select {
		case <-timeout:
			fmt.Println("\n✗ 等待任务完成超时")
			return

		case <-ticker.C:
			result, err := worker.GetTask(ctx, taskID)
			if err != nil {
				fmt.Printf("✗ 获取任务状态失败: %v\n", err)
				return
			}

			// 只在状态改变时输出
			if string(result.TaskStatus) != lastStatus || result.Redo != lastRedo {
				fmt.Printf("\n[监控] 状态: %s | 重试次数: %d/%d\n",
					result.TaskStatus, result.Redo, result.MaxRedo)
				lastStatus = string(result.TaskStatus)
				lastRedo = result.Redo
			}

			// 检查任务是否完成
			if result.IsFinished() {
				fmt.Println("\n========================================")
				fmt.Println("任务执行完成")
				fmt.Println("========================================")
				fmt.Printf("任务 ID: %d\n", result.ID)
				fmt.Printf("最终状态: %s\n", result.TaskStatus)
				fmt.Printf("总尝试次数: %d\n", atomic.LoadInt32(&attemptCount))
				fmt.Printf("重试次数: %d\n", result.Redo)
				fmt.Printf("执行耗时: %d 秒\n", result.Cost)

				if result.IsSuccess() {
					fmt.Println("\n✓ 任务最终执行成功！")
					fmt.Printf("  说明: 任务前 %d 次失败，经过 %d 次重试后成功\n",
						payload.FailTimes, result.Redo)
				} else {
					fmt.Printf("\n✗ 任务最终失败: %s\n", result.ErrMsg)
					fmt.Printf("  说明: 任务在 %d 次尝试后仍然失败\n",
						atomic.LoadInt32(&attemptCount))
				}

				fmt.Println("\n========================================")
				fmt.Println("重试机制要点")
				fmt.Println("========================================")
				fmt.Println("1. 失败的任务会自动重新推入队列")
				fmt.Println("2. 每次重试时 Redo 计数会增加")
				fmt.Println("3. 当 Redo >= MaxRedo 时不再重试")
				fmt.Println("4. OnFailure 回调在每次失败时都会被调用")
				fmt.Println("5. OnSuccess 回调只在最终成功时调用一次")

				fmt.Println("\n按 Ctrl+C 退出...")
				quit := make(chan os.Signal, 1)
				signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
				<-quit

				return
			}
		}
	}
}
