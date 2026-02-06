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

// DemoPayload 演示任务的参数结构
type DemoPayload struct {
	Message string `json:"message"`
	UserID  int    `json:"user_id"`
}

// DemoTaskExecutor 演示任务执行器
type DemoTaskExecutor struct {
	task.BaseExecutor
	payload DemoPayload
}

// Prepare 初始化执行器
func (e *DemoTaskExecutor) Prepare(ctx context.Context, taskEntity *task.TaskEntity) error {
	// 调用基类 Prepare
	if err := e.BaseExecutor.Prepare(ctx, taskEntity); err != nil {
		return err
	}

	// 解析任务参数
	if err := json.Unmarshal([]byte(taskEntity.Payload), &e.payload); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("✓ Prepare: 任务 %d 已初始化，参数: %+v\n", taskEntity.ID, e.payload)
	return nil
}

// Execute 执行任务
func (e *DemoTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("→ Execute: 开始执行任务 %d\n", e.Task.ID)
	fmt.Printf("  处理消息: %s\n", e.payload.Message)
	fmt.Printf("  用户 ID: %d\n", e.payload.UserID)

	// 模拟任务处理
	time.Sleep(2 * time.Second)

	fmt.Printf("✓ Execute: 任务 %d 执行完成\n", e.Task.ID)
	return nil
}

// OnSuccess 成功回调
func (e *DemoTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("✓ OnSuccess: 任务 %d 执行成功\n", e.Task.ID)
	// 这里可以执行事务性操作，如更新数据库
	return nil
}

// OnFailure 失败回调
func (e *DemoTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("✗ OnFailure: 任务 %d 执行失败\n", e.Task.ID)
	// 这里可以执行清理操作或记录日志
	return nil
}

// setupDB 使用 gorm 原生方式创建数据库连接
func setupDB() (*gorm.DB, error) {
	// MySQL DSN 格式: user:pass@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	dsn := "root:123456@tcp(localhost:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local"

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
	fmt.Println("Task 包 - 基本使用示例")
	fmt.Println("========================================\n")

	// 1. 配置 Worker
	fmt.Println("步骤 1: 配置 Worker")
	config := &task.TaskConfig{
		WorkerID:          "demo-worker-001", // Worker 唯一标识
		MaxConcurrency:    3,                 // 最大并发数
		Timeout:           10 * time.Minute,
		MaxRedo:           3,
		RedisKeyPrefix:    "task:demo:",
		EnableHealthCheck: true,
		HealthCheckPeriod: 30 * time.Second,
	}
	fmt.Printf("  WorkerID: %s\n", config.WorkerID)
	fmt.Printf("  MaxConcurrency: %d\n", config.MaxConcurrency)
	fmt.Printf("  MaxRedo: %d\n\n", config.MaxRedo)

	// 2. 创建 Worker
	fmt.Println("步骤 2: 创建 Worker")
	db, err := setupDB()
	if err != nil {
		fmt.Printf("✗ 数据库连接失败: %v\n", err)
		fmt.Println("\n提示: 请确保 MySQL 服务正在运行")
		os.Exit(1)
	}
	if err := task.Init(db); err != nil {
		fmt.Printf("✗ 初始化 Task 包失败: %v\n", err)
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
	fmt.Println("  ✓ Worker 创建成功\n")

	// 3. 注册任务执行器
	fmt.Println("步骤 3: 注册任务执行器")
	worker.RegisterExecutor("demo_task", func() task.TaskExecutor {
		return &DemoTaskExecutor{}
	})
	fmt.Println("  ✓ 已注册执行器: demo_task\n")

	// 4. 启动 Worker
	fmt.Println("步骤 4: 启动 Worker")
	ctx := context.Background()
	if err := worker.Start(ctx); err != nil {
		fmt.Printf("✗ 启动 Worker 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("  ✓ Worker 已启动\n")

	// 延迟关闭 Worker
	defer func() {
		fmt.Println("\n步骤 7: 停止 Worker")
		if err := worker.Stop(ctx); err != nil {
			fmt.Printf("✗ 停止 Worker 失败: %v\n", err)
		} else {
			fmt.Println("  ✓ Worker 已停止")
		}
	}()

	// 5. 创建任务
	fmt.Println("步骤 5: 创建任务")
	payload := DemoPayload{
		Message: "这是一个演示任务",
		UserID:  12345,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("✗ 序列化参数失败: %v\n", err)
		os.Exit(1)
	}

	taskEntity := &task.TaskEntity{
		TaskType:    "demo_task",
		SubjectType: "demo",
		SubjectID:   1,
		Payload:     string(payloadJSON),
		Timeout:     5 * time.Minute,
		MaxRedo:     3,
		Priority:    0,
		CompanyID:   1,
		Uin:         1001,
	}
	taskEntityList := []*task.TaskEntity{taskEntity}

	if err := worker.CreateTasks(ctx, taskEntityList); err != nil {
		fmt.Printf("✗ 创建任务失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("  ✓ 任务已创建，ID: %d\n\n", taskEntity.ID)

	// 6. 等待任务完成
	fmt.Println("步骤 6: 等待任务完成")
	fmt.Println("========================================\n")

	// 轮询检查任务状态
	taskID := taskEntity.ID
	timeout := time.After(30 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

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

			// 检查任务是否完成
			if result.IsFinished() {
				fmt.Println("\n========================================")
				fmt.Println("任务执行结果")
				fmt.Println("========================================")
				fmt.Printf("任务 ID: %d\n", result.ID)
				fmt.Printf("任务类型: %s\n", result.TaskType)
				fmt.Printf("任务状态: %s\n", result.TaskStatus)
				fmt.Printf("重试次数: %d/%d\n", result.Redo, result.MaxRedo)
				fmt.Printf("执行耗时: %d 秒\n", result.Cost)

				if result.IsSuccess() {
					fmt.Println("\n✓ 任务执行成功！")
				} else {
					fmt.Printf("\n✗ 任务执行失败: %s\n", result.ErrMsg)
				}

				fmt.Println("\n按 Ctrl+C 退出...")

				// 等待退出信号
				quit := make(chan os.Signal, 1)
				signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
				<-quit

				return
			}

			// 显示当前状态
			fmt.Printf("  任务状态: %s (已重试 %d 次)\n", result.TaskStatus, result.Redo)
		}
	}
}
