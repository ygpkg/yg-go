package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/ygpkg/yg-go/task/manager"
	"github.com/ygpkg/yg-go/task/model"
	"github.com/ygpkg/yg-go/task/worker"
)

// ===== 场景共享状态 =====

// ConcurrentScenarioState 并发场景的共享状态
type ConcurrentScenarioState struct {
	executingCount int32
	completedCount int32
	mu             sync.Mutex
	startTimes     map[int]time.Time
}

// StepsScenarioState 步骤场景的共享状态
type StepsScenarioState struct {
	executionOrder []int
	mu             sync.Mutex
}

var (
	concurrentState       *ConcurrentScenarioState
	stepsState            *StepsScenarioState
	mixedConcurrencyStats *TaskStats
	retryAttemptCount     int32
)

// ===== 执行器注册函数 =====

// registerBasicExecutors 注册基本示例的执行器
func registerBasicExecutors(w *worker.Worker) {
	w.RegisterExecutor("demo_task", func(payload string) (worker.TaskExecutor, error) {
		return NewDemoTaskExecutor(payload)
	})
}

// registerRetryExecutors 注册重试示例的执行器
func registerRetryExecutors(w *worker.Worker) {
	// 重置共享计数器
	atomic.StoreInt32(&retryAttemptCount, 0)

	w.RegisterExecutor("retry_task", func(payload string) (worker.TaskExecutor, error) {
		return NewRetryTaskExecutor(payload, &retryAttemptCount)
	})
}

// registerTimeoutExecutors 注册超时示例的执行器
func registerTimeoutExecutors(w *worker.Worker) {
	w.RegisterExecutor("timeout_task", func(payload string) (worker.TaskExecutor, error) {
		return NewTimeoutTaskExecutor(payload)
	})
}

// registerConcurrentExecutors 注册并发示例的执行器
func registerConcurrentExecutors(w *worker.Worker) {
	// 初始化共享状态
	concurrentState = &ConcurrentScenarioState{
		startTimes: make(map[int]time.Time),
	}

	w.RegisterExecutor("concurrent_task", func(payload string) (worker.TaskExecutor, error) {
		return NewConcurrentTaskExecutor(payload,
			&concurrentState.executingCount,
			&concurrentState.completedCount,
			&concurrentState.mu,
			&concurrentState.startTimes)
	})
}

// registerStepsExecutors 注册步骤示例的执行器
func registerStepsExecutors(w *worker.Worker) {
	// 初始化共享状态
	stepsState = &StepsScenarioState{
		executionOrder: make([]int, 0),
	}

	w.RegisterExecutor("step_task", func(payload string) (worker.TaskExecutor, error) {
		return NewStepTaskExecutor(payload, &stepsState.executionOrder, &stepsState.mu)
	})
}

// registerMixedConcurrencyExecutors 注册混合并发示例的执行器
func registerMixedConcurrencyExecutors(w *worker.Worker) {
	// 初始化共享统计
	mixedConcurrencyStats = NewTaskStats()

	w.RegisterExecutor("fast_task", func(payload string) (worker.TaskExecutor, error) {
		return NewFastTaskExecutor(payload, mixedConcurrencyStats)
	}, worker.WithConcurrency(10))

	w.RegisterExecutor("slow_task", func(payload string) (worker.TaskExecutor, error) {
		return NewSlowTaskExecutor(payload, mixedConcurrencyStats)
	}, worker.WithConcurrency(2))

	w.RegisterExecutor("api_task", func(payload string) (worker.TaskExecutor, error) {
		return NewApiTaskExecutor(payload, mixedConcurrencyStats)
	}, worker.WithConcurrency(5))

	w.RegisterExecutor("default_task", func(payload string) (worker.TaskExecutor, error) {
		return NewDefaultTaskExecutor(payload, mixedConcurrencyStats)
	})
}

// ===== 场景执行函数 =====

// runBasicScenario 基本任务创建和执行
func runBasicScenario(ctx context.Context, mgr *manager.Manager, w *worker.Worker) error {
	printSection("基本任务创建和执行")

	fmt.Println("[DEBUG] Worker 已启动，准备创建任务...")
	time.Sleep(500 * time.Millisecond)

	// 创建任务
	fmt.Println("创建任务...")
	payload := DemoPayload{
		Message: "这是一个演示任务",
		UserID:  12345,
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化参数失败: %w", err)
	}

	taskEntity := &model.TaskEntity{
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

	fmt.Printf("[DEBUG] 准备调用 CreateTasks，任务类型: %s\n", taskEntity.TaskType)
	if err := mgr.CreateTasks(ctx, []*model.TaskEntity{taskEntity}); err != nil {
		fmt.Printf("[DEBUG] CreateTasks 调用失败: %v\n", err)
		return fmt.Errorf("创建任务失败: %w", err)
	}
	fmt.Printf("✓ 任务已创建，ID: %d\n", taskEntity.ID)
	fmt.Printf("[DEBUG] 任务入队成功，等待 Worker 获取并执行...\n\n")

	// 等待任务完成
	return waitForTaskCompletion(ctx, mgr, taskEntity.ID, 30*time.Second)
}

// runRetryScenario 任务重试机制演示
func runRetryScenario(ctx context.Context, mgr *manager.Manager, w *worker.Worker) error {
	printSection("任务重试机制演示")

	// 创建任务
	fmt.Println("创建重试任务...")
	fmt.Println("配置: 前 2 次尝试失败，第 3 次成功")
	fmt.Println()

	payload := RetryPayload{
		Message:    "重试演示任务",
		FailTimes:  2,
		FailReason: "模拟失败",
	}

	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("序列化参数失败: %w", err)
	}

	taskEntity := &model.TaskEntity{
		TaskType:    "retry_task",
		SubjectType: "retry_demo",
		SubjectID:   1,
		Payload:     string(payloadJSON),
		Timeout:     5 * time.Minute,
		MaxRedo:     3,
		Priority:    0,
		CompanyID:   1,
		Uin:         1001,
	}

	if err := mgr.CreateTasks(ctx, []*model.TaskEntity{taskEntity}); err != nil {
		return fmt.Errorf("创建任务失败: %w", err)
	}
	fmt.Printf("✓ 任务已创建，ID: %d\n\n", taskEntity.ID)

	// 等待任务完成（需要更长时间）
	return waitForTaskCompletion(ctx, mgr, taskEntity.ID, 60*time.Second)
}

// runTimeoutScenario 任务超时处理演示
func runTimeoutScenario(ctx context.Context, mgr *manager.Manager, w *worker.Worker) error {
	printSection("任务超时处理演示")

	// 创建两个任务：一个超时，一个不超时
	fmt.Println("创建两个任务:")
	fmt.Println("  任务 1: 执行 3 秒，超时 5 秒（不会超时，会检查上下文）")
	fmt.Println("  任务 2: 执行 8 秒，超时 5 秒（会超时，不检查上下文）")
	fmt.Println()

	tasks := []*model.TaskEntity{
		{
			TaskType:    "timeout_task",
			SubjectType: "timeout_demo",
			SubjectID:   1,
			Payload: mustMarshal(TimeoutPayload{
				Message:      "任务 1：不会超时",
				Duration:     3,
				CheckContext: true,
			}),
			Timeout:   5 * time.Second,
			MaxRedo:   0,
			Priority:  0,
			CompanyID: 1,
			Uin:       1001,
		},
		{
			TaskType:    "timeout_task",
			SubjectType: "timeout_demo",
			SubjectID:   2,
			Payload: mustMarshal(TimeoutPayload{
				Message:      "任务 2：会超时",
				Duration:     8,
				CheckContext: false,
			}),
			Timeout:   5 * time.Second,
			MaxRedo:   0,
			Priority:  0,
			CompanyID: 1,
			Uin:       1001,
		},
	}

	if err := mgr.CreateTasks(ctx, tasks); err != nil {
		return fmt.Errorf("创建任务失败: %w", err)
	}
	fmt.Printf("✓ 已创建 %d 个任务\n\n", len(tasks))

	// 等待所有任务完成
	taskIDs := []uint{tasks[0].ID, tasks[1].ID}
	return waitForMultipleTasksCompletion(ctx, mgr, taskIDs, 60*time.Second)
}

// runConcurrentScenario 并发任务处理演示
func runConcurrentScenario(ctx context.Context, mgr *manager.Manager, w *worker.Worker) error {
	printSection("并发任务处理演示")

	// 批量创建任务
	taskCount := 20
	fmt.Printf("创建 %d 个并发任务（每个耗时 500ms）...\n", taskCount)

	var tasks []*model.TaskEntity
	for i := 1; i <= taskCount; i++ {
		payload := ConcurrentPayload{
			Index:    i,
			Message:  fmt.Sprintf("并发任务 %d", i),
			Duration: 500,
		}

		payloadJSON, _ := json.Marshal(payload)

		tasks = append(tasks, &model.TaskEntity{
			TaskType:    "concurrent_task",
			SubjectType: "concurrent_demo",
			SubjectID:   uint(i),
			Payload:     string(payloadJSON),
			Timeout:     5 * time.Minute,
			MaxRedo:     3,
			Priority:    0,
			CompanyID:   1,
			Uin:         1001,
		})
	}

	startTime := time.Now()
	if err := mgr.CreateTasks(ctx, tasks); err != nil {
		return fmt.Errorf("批量创建任务失败: %w", err)
	}
	fmt.Printf("✓ 已创建 %d 个任务\n\n", len(tasks))

	// 实时统计
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				executing := atomic.LoadInt32(&concurrentState.executingCount)
				completed := atomic.LoadInt32(&concurrentState.completedCount)

				if completed >= int32(taskCount) {
					return
				}

				fmt.Printf("[统计] 正在执行: %d | 已完成: %d/%d\n",
					executing, completed, taskCount)
			}
		}
	}()

	// 等待所有任务完成
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("等待任务完成超时")

		case <-ticker.C:
			completed := atomic.LoadInt32(&concurrentState.completedCount)
			if completed >= int32(taskCount) {
				totalTime := time.Since(startTime)

				fmt.Println("\n========================================")
				fmt.Println("任务执行完成")
				fmt.Println("========================================")
				fmt.Printf("任务总数: %d\n", taskCount)
				fmt.Printf("总耗时: %v\n", totalTime.Round(time.Millisecond))
				fmt.Printf("平均每个任务: %v\n",
					(totalTime / time.Duration(taskCount)).Round(time.Millisecond))

				serialTime := float64(taskCount) * 0.5
				speedup := serialTime / totalTime.Seconds()
				fmt.Printf("\n性能分析:\n")
				fmt.Printf("  串行执行预计: %.1f 秒\n", serialTime)
				fmt.Printf("  实际耗时: %.1f 秒\n", totalTime.Seconds())
				fmt.Printf("  加速比: %.2fx\n", speedup)

				return waitForExit()
			}
		}
	}
}

// runStepsScenario 步骤化任务流程演示
func runStepsScenario(ctx context.Context, mgr *manager.Manager, w *worker.Worker) error {
	printSection("步骤化任务流程演示")

	// 创建步骤化任务
	orderID := 1001
	appGroup := fmt.Sprintf("order_%d", orderID)

	fmt.Printf("创建订单 %d 的处理流程（3 个步骤）...\n", orderID)
	fmt.Println("  步骤 1: 验证订单")
	fmt.Println("  步骤 2: 处理支付")
	fmt.Println("  步骤 3: 发货")
	fmt.Println()

	steps := []struct {
		step        int
		name        string
		description string
	}{
		{1, "验证订单", "检查订单信息和库存"},
		{2, "处理支付", "扣款并生成支付记录"},
		{3, "发货", "生成物流单并发货"},
	}

	var tasks []*model.TaskEntity
	for _, s := range steps {
		payload := StepPayload{
			StepName:    s.name,
			OrderID:     orderID,
			Description: s.description,
			Step:        s.step,
			AppGroup:    appGroup,
		}

		payloadJSON, _ := json.Marshal(payload)

		tasks = append(tasks, &model.TaskEntity{
			TaskType:    "step_task",
			SubjectType: "order",
			SubjectID:   uint(orderID),
			AppGroup:    appGroup,
			Step:        s.step,
			Payload:     string(payloadJSON),
			Timeout:     5 * time.Minute,
			MaxRedo:     3,
			Priority:    0,
			CompanyID:   1,
			Uin:         1001,
		})
	}

	if err := mgr.CreateTasks(ctx, tasks); err != nil {
		return fmt.Errorf("创建任务失败: %w", err)
	}
	fmt.Printf("✓ 已创建 %d 个步骤任务\n\n", len(tasks))

	// 等待所有步骤完成
	taskIDs := make([]uint, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}

	if err := waitForMultipleTasksCompletion(ctx, mgr, taskIDs, 60*time.Second); err != nil {
		return err
	}

	// 验证执行顺序
	fmt.Println("\n========================================")
	fmt.Println("执行顺序验证")
	fmt.Println("========================================")
	fmt.Printf("实际执行顺序: %v\n", stepsState.executionOrder)

	isOrdered := true
	for i := 0; i < len(stepsState.executionOrder)-1; i++ {
		if stepsState.executionOrder[i] > stepsState.executionOrder[i+1] {
			isOrdered = false
			break
		}
	}

	if isOrdered {
		fmt.Println("✓ 步骤按正确顺序执行")
	} else {
		fmt.Println("✗ 步骤执行顺序错误")
	}

	return waitForExit()
}

// runMixedConcurrencyScenario 混合并发任务演示
func runMixedConcurrencyScenario(ctx context.Context, mgr *manager.Manager, w *worker.Worker) error {
	printSection("混合并发任务演示")

	fmt.Println("已注册的执行器:")
	fmt.Println("  ✓ fast_task - 并发数: 10 (快速任务，高并发)")
	fmt.Println("  ✓ slow_task - 并发数: 2 (慢速任务，低并发)")
	fmt.Println("  ✓ api_task - 并发数: 5 (API调用，中等并发)")
	fmt.Println("  ✓ default_task - 并发数: 3 (使用全局默认值)")
	fmt.Println()

	// 创建混合任务
	fmt.Println("创建混合任务:")
	var allTasks []*model.TaskEntity

	// 创建 10 个快速任务（100ms）
	fmt.Println("  - 10 个快速任务（每个 100ms）")
	for i := 1; i <= 10; i++ {
		payload := FastTaskPayload{
			Index:    i,
			Message:  fmt.Sprintf("快速任务 %d", i),
			Duration: 100,
		}
		payloadJSON, _ := json.Marshal(payload)
		allTasks = append(allTasks, &model.TaskEntity{
			TaskType:    "fast_task",
			SubjectType: "fast_demo",
			SubjectID:   uint(i),
			Payload:     string(payloadJSON),
			Timeout:     5 * time.Minute,
			MaxRedo:     3,
			Priority:    0,
			CompanyID:   1,
			Uin:         1001,
		})
	}

	// 创建 5 个慢速任务（2000ms）
	fmt.Println("  - 5 个慢速任务（每个 2000ms）")
	for i := 1; i <= 5; i++ {
		payload := SlowTaskPayload{
			Index:    i,
			Message:  fmt.Sprintf("慢速任务 %d", i),
			Duration: 2000,
		}
		payloadJSON, _ := json.Marshal(payload)
		allTasks = append(allTasks, &model.TaskEntity{
			TaskType:    "slow_task",
			SubjectType: "slow_demo",
			SubjectID:   uint(i),
			Payload:     string(payloadJSON),
			Timeout:     5 * time.Minute,
			MaxRedo:     3,
			Priority:    0,
			CompanyID:   1,
			Uin:         1001,
		})
	}

	// 创建 8 个 API 任务（500ms）
	fmt.Println("  - 8 个 API 任务（每个 500ms）")
	for i := 1; i <= 8; i++ {
		payload := ApiTaskPayload{
			Index:    i,
			Endpoint: fmt.Sprintf("https://api.example.com/endpoint/%d", i),
			Duration: 500,
		}
		payloadJSON, _ := json.Marshal(payload)
		allTasks = append(allTasks, &model.TaskEntity{
			TaskType:    "api_task",
			SubjectType: "api_demo",
			SubjectID:   uint(i),
			Payload:     string(payloadJSON),
			Timeout:     5 * time.Minute,
			MaxRedo:     3,
			Priority:    0,
			CompanyID:   1,
			Uin:         1001,
		})
	}

	// 创建 6 个默认任务（300ms）
	fmt.Println("  - 6 个默认任务（每个 300ms）")
	for i := 1; i <= 6; i++ {
		payload := DefaultTaskPayload{
			Index:    i,
			Message:  fmt.Sprintf("默认任务 %d", i),
			Duration: 300,
		}
		payloadJSON, _ := json.Marshal(payload)
		allTasks = append(allTasks, &model.TaskEntity{
			TaskType:    "default_task",
			SubjectType: "default_demo",
			SubjectID:   uint(i),
			Payload:     string(payloadJSON),
			Timeout:     5 * time.Minute,
			MaxRedo:     3,
			Priority:    0,
			CompanyID:   1,
			Uin:         1001,
		})
	}

	// 批量创建任务
	startTime := time.Now()
	if err := mgr.CreateTasks(ctx, allTasks); err != nil {
		return fmt.Errorf("批量创建任务失败: %w", err)
	}
	fmt.Printf("\n✓ 已创建 %d 个任务\n", len(allTasks))
	fmt.Println("  - 快速任务: 10 个 (并发: 10)")
	fmt.Println("  - 慢速任务: 5 个 (并发: 2)")
	fmt.Println("  - API任务: 8 个 (并发: 5)")
	fmt.Println("  - 默认任务: 6 个 (并发: 3)")
	fmt.Println()

	totalTasks := int32(len(allTasks))

	// 实时统计
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fast := atomic.LoadInt32(&mixedConcurrencyStats.fastCompleted)
				slow := atomic.LoadInt32(&mixedConcurrencyStats.slowCompleted)
				api := atomic.LoadInt32(&mixedConcurrencyStats.apiCompleted)
				def := atomic.LoadInt32(&mixedConcurrencyStats.defaultCompleted)
				total := fast + slow + api + def

				if total >= totalTasks {
					return
				}

				fmt.Printf("[统计] 快速: %d/10 | 慢速: %d/5 | API: %d/8 | 默认: %d/6 | 总计: %d/%d\n",
					fast, slow, api, def, total, totalTasks)
			}
		}
	}()

	// 等待所有任务完成
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return fmt.Errorf("等待任务完成超时")

		case <-ticker.C:
			fast := atomic.LoadInt32(&mixedConcurrencyStats.fastCompleted)
			slow := atomic.LoadInt32(&mixedConcurrencyStats.slowCompleted)
			api := atomic.LoadInt32(&mixedConcurrencyStats.apiCompleted)
			def := atomic.LoadInt32(&mixedConcurrencyStats.defaultCompleted)
			total := fast + slow + api + def

			if total >= totalTasks {
				totalTime := time.Since(startTime)

				fmt.Println("\n========================================")
				fmt.Println("任务执行完成")
				fmt.Println("========================================")
				fmt.Printf("总耗时: %v\n", totalTime.Round(time.Millisecond))
				fmt.Printf("\n任务完成情况:\n")
				fmt.Printf("  快速任务: %d/10\n", fast)
				fmt.Printf("  慢速任务: %d/5\n", slow)
				fmt.Printf("  API任务: %d/8\n", api)
				fmt.Printf("  默认任务: %d/6\n", def)
				fmt.Printf("  总计: %d/%d\n", total, totalTasks)

				fmt.Println("\n========================================")
				fmt.Println("混合并发要点说明")
				fmt.Println("========================================")
				fmt.Println("1. 不同任务类型可以配置不同的并发数")
				fmt.Println("2. 使用 w.WithConcurrency() 选项指定并发数")
				fmt.Println("3. 不传选项时使用全局默认值（向后兼容）")
				fmt.Println("4. 快速任务高并发可提升吞吐量")
				fmt.Println("5. 慢速任务低并发避免资源耗尽")
				fmt.Println("6. 根据任务特性合理配置并发数")

				return waitForExit()
			}
		}
	}
}

// ===== 辅助函数 =====

// waitForTaskCompletion 等待单个任务完成
func waitForTaskCompletion(ctx context.Context, mgr *manager.Manager, taskID uint, timeout time.Duration) error {
	fmt.Println("等待任务完成...")
	fmt.Println("========================================")
	fmt.Println("[DEBUG] 开始轮询任务状态...")
	fmt.Println()

	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCh:
			fmt.Println("[DEBUG] 等待任务完成超时！")
			return fmt.Errorf("等待任务完成超时")

		case <-ticker.C:
			fmt.Printf("[DEBUG] 查询任务 %d 状态...\n", taskID)
			result, err := mgr.GetTask(ctx, taskID)
			if err != nil {
				fmt.Printf("[DEBUG] GetTask 调用失败: %v\n", err)
				return fmt.Errorf("获取任务状态失败: %w", err)
			}

			fmt.Printf("[DEBUG] 任务状态: %s, WorkerID: %s, Redo: %d/%d\n",
				result.TaskStatus, result.WorkerID, result.Redo, result.MaxRedo)

			if result.IsFinished() {
				fmt.Println("\n========================================")
				fmt.Println("任务执行结果")
				fmt.Println("========================================")
				fmt.Printf("任务 ID: %d\n", result.ID)
				fmt.Printf("任务类型: %s\n", result.TaskType)
				fmt.Printf("任务状态: %s\n", result.TaskStatus)
				fmt.Printf("重试次数: %d/%d\n", result.Redo, result.MaxRedo)
				fmt.Printf("执行耗时: %d 秒\n", result.Cost)
				fmt.Printf("执行 Worker: %s\n", result.WorkerID)

				if result.IsSuccess() {
					fmt.Println("\n✓ 任务执行成功！")
				} else {
					fmt.Printf("\n✗ 任务执行失败: %s\n", result.ErrMsg)
				}

				fmt.Println("[DEBUG] 任务已完成，等待用户退出...")
				return waitForExit()
			}

			fmt.Printf("  任务状态: %s (已重试 %d 次)\n", result.TaskStatus, result.Redo)
		}
	}
}

// waitForMultipleTasksCompletion 等待多个任务完成
func waitForMultipleTasksCompletion(ctx context.Context, mgr *manager.Manager, taskIDs []uint, timeout time.Duration) error {
	fmt.Println("等待所有任务完成...")

	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	completed := make(map[uint]bool)

	for {
		select {
		case <-timeoutCh:
			return fmt.Errorf("等待任务完成超时")

		case <-ticker.C:
			allFinished := true
			for _, taskID := range taskIDs {
				if completed[taskID] {
					continue
				}

				result, err := mgr.GetTask(ctx, taskID)
				if err != nil {
					return fmt.Errorf("获取任务 %d 状态失败: %w", taskID, err)
				}

				if result.IsFinished() {
					completed[taskID] = true
					status := "成功"
					if !result.IsSuccess() {
						status = "失败"
					}
					fmt.Printf("  任务 %d %s\n", taskID, status)
				} else {
					allFinished = false
				}
			}

			if allFinished {
				fmt.Println("\n✓ 所有任务已完成")
				return nil
			}
		}
	}
}

// waitForExit 等待用户按 Ctrl+C 退出
func waitForExit() error {
	fmt.Println("\n按 Ctrl+C 退出...")
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	return nil
}

// mustMarshal 序列化 JSON，失败时 panic
func mustMarshal(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}
