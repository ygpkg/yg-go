package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ygpkg/yg-go/task"
)

func main() {
	printBanner()

	// 1. 显示菜单并获取用户选择（先选择，避免不必要的连接）
	choice := showMenuAndGetChoice()

	if choice == 0 {
		fmt.Println("再见！")
		return
	}

	// 2. 初始化数据库和 Redis
	fmt.Println("正在连接数据库和 Redis...")
	db, redisClient, err := setupInfra()
	if err != nil {
		fmt.Printf("✗ 初始化失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ 数据库和 Redis 连接成功")

	// 初始化 Task 包
	if err := task.Init(db); err != nil {
		fmt.Printf("✗ 初始化 Task 包失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Task 包初始化完成")
	fmt.Println()

	// 3. 根据选择创建对应配置的 Worker
	config := createWorkerConfig(choice)
	worker, err := task.NewWorker(config, db, redisClient)
	if err != nil {
		fmt.Printf("✗ 创建 Worker 失败: %v\n", err)
		os.Exit(1)
	}

	ctx := context.Background()

	// 4. 根据场景注册执行器（必须在 Worker.Start() 之前）
	switch choice {
	case 1:
		registerBasicExecutors(worker)
	case 2:
		registerRetryExecutors(worker)
	case 3:
		registerTimeoutExecutors(worker)
	case 4:
		registerConcurrentExecutors(worker)
	case 5:
		registerStepsExecutors(worker)
	case 6:
		registerMixedConcurrencyExecutors(worker)
	default:
		fmt.Println("无效的选项")
		return
	}

	// 5. 启动 Worker
	if err := worker.Start(ctx); err != nil {
		fmt.Printf("✗ 启动 Worker 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Worker 已启动")
	fmt.Println()

	// 延迟停止 Worker
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

	// 6. 运行对应场景
	var scenarioErr error
	switch choice {
	case 1:
		scenarioErr = runBasicScenario(ctx, worker)
	case 2:
		scenarioErr = runRetryScenario(ctx, worker)
	case 3:
		scenarioErr = runTimeoutScenario(ctx, worker)
	case 4:
		scenarioErr = runConcurrentScenario(ctx, worker)
	case 5:
		scenarioErr = runStepsScenario(ctx, worker)
	case 6:
		scenarioErr = runMixedConcurrencyScenario(ctx, worker)
	default:
		fmt.Println("无效的选项")
		return
	}

	if scenarioErr != nil {
		fmt.Printf("\n✗ 场景执行失败: %v\n", scenarioErr)
		os.Exit(1)
	}
}

// printBanner 打印横幅
func printBanner() {
	fmt.Println("========================================")
	fmt.Println("         Task 包使用示例")
	fmt.Println("========================================")
	fmt.Println()
}

// showMenuAndGetChoice 显示菜单并获取用户选择
func showMenuAndGetChoice() int {
	fmt.Println("请选择要运行的示例：")
	fmt.Println()
	fmt.Println("  1. 基本任务创建和执行")
	fmt.Println("  2. 任务重试机制")
	fmt.Println("  3. 任务超时处理")
	fmt.Println("  4. 并发任务处理")
	fmt.Println("  5. 步骤化任务流程")
	fmt.Println("  6. 混合并发（不同任务类型不同并发数）")
	fmt.Println("  0. 退出")
	fmt.Println()
	fmt.Print("请输入选项 (0-6): ")

	var choice int
	_, err := fmt.Scanf("%d", &choice)
	if err != nil {
		fmt.Println("输入无效，请输入数字")
		return showMenuAndGetChoice()
	}

	if choice < 0 || choice > 6 {
		fmt.Println("无效的选项，请重新选择")
		fmt.Println()
		return showMenuAndGetChoice()
	}

	fmt.Println()
	return choice
}

// createWorkerConfig 根据场景创建 Worker 配置
func createWorkerConfig(scenario int) *task.TaskConfig {
	baseConfig := &task.TaskConfig{
		Timeout:           10 * time.Minute,
		MaxRedo:           3,
		MaxConcurrency:    5,
		QueueBlockTime:    5 * time.Second,
		RedisKeyPrefix:    "task:example:",
		EnableHealthCheck: true,
		HealthCheckPeriod: 30 * time.Second,
	}

	switch scenario {
	case 1: // 基本示例
		baseConfig.WorkerID = "basic-worker-001"
		baseConfig.MaxConcurrency = 3
	case 2: // 重试示例
		baseConfig.WorkerID = "retry-worker-001"
		baseConfig.MaxConcurrency = 1
	case 3: // 超时示例
		baseConfig.WorkerID = "timeout-worker-001"
		baseConfig.MaxConcurrency = 2
	case 4: // 并发示例
		baseConfig.WorkerID = "concurrent-worker-001"
		baseConfig.MaxConcurrency = 5
	case 5: // 步骤示例
		baseConfig.WorkerID = "steps-worker-001"
		baseConfig.MaxConcurrency = 3
	case 6: // 混合并发示例
		baseConfig.WorkerID = "mixed-worker-001"
		baseConfig.MaxConcurrency = 3 // 默认并发数
	default:
		baseConfig.WorkerID = "example-worker-001"
	}

	return baseConfig
}
