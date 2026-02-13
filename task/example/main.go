package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/ygpkg/yg-go/task/health"
	"github.com/ygpkg/yg-go/task/manager"
	"github.com/ygpkg/yg-go/task/worker"
)

// WorkManagerAdapter 适配器实现 worker.WorkManager 接口
type WorkManagerAdapter struct {
	mgr *manager.Manager
}

func (a *WorkManagerAdapter) GetNextTask(ctx context.Context, taskType, workerID string) (worker.TaskInfo, error) {
	taskEntity, err := a.mgr.GetNextTask(ctx, taskType, workerID)
	if err != nil {
		return worker.TaskInfo{}, err
	}

	return worker.TaskInfo{
		TaskID:    taskEntity.ID,
		TaskType:  taskEntity.TaskType,
		Payload:   taskEntity.Payload,
		Timeout:   taskEntity.Timeout,
		AppGroup:  taskEntity.AppGroup,
		SubjectID: taskEntity.SubjectID,
		Redo:      taskEntity.Redo,
		MaxRedo:   taskEntity.MaxRedo,
	}, nil
}

func (a *WorkManagerAdapter) SaveTaskResult(ctx context.Context, workerID string, info worker.TaskInfo, result string, execErr error, onCallback func(context.Context) error) error {
	return a.mgr.SaveTaskResult(ctx, info.TaskID, result, execErr, onCallback)
}

func (a *WorkManagerAdapter) InitTaskDBStatus(ctx context.Context) error {
	return a.mgr.InitTaskDBStatus(ctx)
}

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
	if err := manager.InitDB(db); err != nil {
		fmt.Printf("✗ 初始化 Task 包失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Task 包初始化完成")
	fmt.Println()

	ctx := context.Background()

	// 3. 创建任务管理器
	// 注意：这里的 taskMgr 是直接连接数据库和 Redis 的实现。
	// 在实际生产环境中，如果 Worker 是单独部署的，这里的 taskMgr 可能会替换为
	// 通过 HTTP 或 RPC 调用中心化任务管理服务的实现。
	managerConfig := createManagerConfig(choice)
	taskMgr, err := manager.NewManager(managerConfig, db, redisClient)
	if err != nil {
		fmt.Printf("✗ 创建任务管理器失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ 任务管理器创建成功")

	// 4. 创建 Worker 适配器
	workMgr := &WorkManagerAdapter{mgr: taskMgr}

	// 5. 创建 Worker
	workerConfig := createWorkerConfig(choice)
	w, err := worker.NewWorker(workerConfig, workMgr)
	if err != nil {
		fmt.Printf("✗ 创建 Worker 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Worker 创建成功")

	// 6. 创建健康检查器（独立运行）
	healthConfig := &health.CheckerConfig{
		KeyPrefix:   "task:example:",
		RedisClient: redisClient,
		CheckPeriod: 30 * time.Second,
		// 定义发现 Worker 死亡时的处理逻辑
		OnWorkerDead: func(ctx context.Context, info health.DeadWorkerInfo) error {
			return handleWorkerDead(ctx, info, taskMgr)
		},
	}
	healthChecker, err := health.NewChecker(healthConfig)
	if err != nil {
		fmt.Printf("✗ 创建健康检查器失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ 健康检查器创建成功")

	// 6. 根据场景注册执行器（必须在 Worker.Start() 之前）
	switch choice {
	case 1:
		registerBasicExecutors(w)
	case 2:
		registerRetryExecutors(w)
	case 3:
		registerTimeoutExecutors(w)
	case 4:
		registerConcurrentExecutors(w)
	case 5:
		registerStepsExecutors(w)
	case 6:
		registerMixedConcurrencyExecutors(w)
	default:
		fmt.Println("无效的选项")
		return
	}

	// 7. 启动健康检查器
	if err := healthChecker.Start(ctx); err != nil {
		fmt.Printf("✗ 启动健康检查器失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ 健康检查器已启动")

	// 8. 启动 Worker
	if err := w.Start(ctx); err != nil {
		fmt.Printf("✗ 启动 Worker 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Worker 已启动")
	fmt.Println()

	// 延迟停止服务
	defer func() {
		fmt.Println("\n========================================")
		fmt.Println("停止服务")
		fmt.Println("========================================")
		if err := w.Stop(ctx); err != nil {
			fmt.Printf("✗ 停止 Worker 失败: %v\n", err)
		} else {
			fmt.Println("✓ Worker 已停止")
		}
		if err := healthChecker.Stop(ctx); err != nil {
			fmt.Printf("✗ 停止健康检查器失败: %v\n", err)
		} else {
			fmt.Println("✓ 健康检查器已停止")
		}
	}()

	// 9. 运行对应场景
	var scenarioErr error
	switch choice {
	case 1:
		scenarioErr = runBasicScenario(ctx, taskMgr, w)
	case 2:
		scenarioErr = runRetryScenario(ctx, taskMgr, w)
	case 3:
		scenarioErr = runTimeoutScenario(ctx, taskMgr, w)
	case 4:
		scenarioErr = runConcurrentScenario(ctx, taskMgr, w)
	case 5:
		scenarioErr = runStepsScenario(ctx, taskMgr, w)
	case 6:
		scenarioErr = runMixedConcurrencyScenario(ctx, taskMgr, w)
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

// createManagerConfig 根据场景创建 Manager 配置
func createManagerConfig(scenario int) *manager.ManagerConfig {
	return &manager.ManagerConfig{
		KeyPrefix:      "task:example:",
		QueueBlockTime: 5 * time.Second,
	}
}

// createWorkerConfig 根据场景创建 Worker 配置
func createWorkerConfig(scenario int) *worker.WorkerConfig {
	baseConfig := &worker.WorkerConfig{
		Timeout:        10 * time.Minute,
		MaxRedo:        3,
		MaxConcurrency: 5,
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

// handleWorkerDead 处理 Worker 死亡事件
func handleWorkerDead(ctx context.Context, info health.DeadWorkerInfo, taskMgr *manager.Manager) error {
	fmt.Printf("! 发现死亡 Worker: %s, 任务ID: %d\n", info.WorkerID, info.TaskID)

	// 使用 SaveTaskResult 方法将任务标记为失败，由框架内部处理重试逻辑
	errMark := fmt.Errorf("worker heartbeat timeout")
	callback := func(ctx context.Context) error {
		fmt.Printf("✓ 死亡 Worker 任务 %d 已通过回调处理\n", info.TaskID)
		return nil
	}

	if err := taskMgr.SaveTaskResult(ctx, info.TaskID, "", errMark, callback); err != nil {
		return fmt.Errorf("failed to save task result: %w", err)
	}

	fmt.Printf("✓ 任务已标记为失败\n")
	fmt.Printf("✓ 任务重新入队逻辑已在 SaveTaskResult 内部处理\n")
	return nil
}
