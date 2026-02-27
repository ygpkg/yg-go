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

	ctx := context.Background()

	fmt.Println("正在连接数据库和 Redis...")
	db, redisClient, err := setupInfra()
	if err != nil {
		fmt.Printf("✗ 初始化失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ 数据库和 Redis 连接成功")

	if err := manager.InitDB(db); err != nil {
		fmt.Printf("✗ 初始化 Task 包失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Task 包初始化完成")
	fmt.Println()

	managerConfig := &manager.ManagerConfig{
		KeyPrefix:      "task:example:",
		QueueBlockTime: 5 * time.Second,
	}
	taskMgr, err := manager.NewManager(managerConfig, db, redisClient)
	if err != nil {
		fmt.Printf("✗ 创建任务管理器失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ 任务管理器创建成功")

	workMgr := &WorkManagerAdapter{mgr: taskMgr}

	workerConfig := &worker.WorkerConfig{
		Timeout:        10 * time.Minute,
		MaxRedo:        3,
		MaxConcurrency: 3,
		WorkerID:       "basic-worker-001",
	}
	w, err := worker.NewWorker(workerConfig, workMgr)
	if err != nil {
		fmt.Printf("✗ 创建 Worker 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Worker 创建成功")

	healthConfig := &health.CheckerConfig{
		KeyPrefix:   "task:example:",
		RedisClient: redisClient,
		CheckPeriod: 30 * time.Second,
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

	registerBasicExecutors(w)

	registerAdditionalExecutors(w, healthChecker)

	if err := taskMgr.Start(ctx); err != nil {
		fmt.Printf("✗ 启动任务管理器失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ 任务管理器已启动")

	if err := healthChecker.Start(ctx); err != nil {
		fmt.Printf("✗ 启动健康检查器失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ 健康检查器已启动")

	if err := w.Start(ctx); err != nil {
		fmt.Printf("✗ 启动 Worker 失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("✓ Worker 已启动")
	fmt.Println()

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
		if err := taskMgr.Stop(ctx); err != nil {
			fmt.Printf("✗ 停止任务管理器失败: %v\n", err)
		} else {
			fmt.Println("✓ 任务管理器已停止")
		}
	}()

	scenarioErr := runBasicScenario(ctx, taskMgr, w, healthChecker)
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

// handleWorkerDead 处理 Worker 死亡事件
func handleWorkerDead(ctx context.Context, info health.DeadWorkerInfo, taskMgr *manager.Manager) error {
	fmt.Printf("! 发现死亡 Worker: %s, 任务ID: %d\n", info.WorkerID, info.TaskID)

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

// registerAdditionalExecutors 注册额外的执行器
func registerAdditionalExecutors(w *worker.Worker, healthChecker *health.Checker) {
	w.RegisterExecutor("timeout_task", func(payload string) (worker.TaskExecutor, error) {
		return NewTimeoutTaskExecutor(payload)
	})
	w.RegisterExecutor("fail_task", func(payload string) (worker.TaskExecutor, error) {
		return NewFailTaskExecutor(payload)
	})
	w.RegisterExecutor("health_task", func(payload string) (worker.TaskExecutor, error) {
		return NewHealthTaskExecutor(payload, healthChecker, "health_task", "health-worker", 0)
	})
}
