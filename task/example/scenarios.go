package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/task/health"
	"github.com/ygpkg/yg-go/task/manager"
	"github.com/ygpkg/yg-go/task/model"
	"github.com/ygpkg/yg-go/task/worker"
)

// registerBasicExecutors 注册基本示例的执行器
func registerBasicExecutors(w *worker.Worker) {
	w.RegisterExecutor("demo_task", func(payload string) (worker.TaskExecutor, error) {
		return NewDemoTaskExecutor(payload)
	})
}

// runBasicScenario 基本任务创建和执行
func runBasicScenario(ctx context.Context, mgr *manager.Manager, w *worker.Worker, healthChecker *health.Checker) error {
	runNormalTaskScenario(ctx, mgr)
	runTimeoutScenario(ctx, mgr)
	runFailureScenario(ctx, mgr)
	runHealthCheckScenario(ctx, mgr, w, healthChecker)
	return nil
}

// runNormalTaskScenario 测试 1：正常任务执行
func runNormalTaskScenario(ctx context.Context, mgr *manager.Manager) {
	printSection("测试 1: 正常任务执行")

	fmt.Println("创建一个正常任务...")
	payload := DemoPayload{
		Message: "这是一个演示任务",
		UserID:  12345,
	}

	payloadJSON, _ := json.Marshal(payload)
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

	if err := mgr.CreateTasks(ctx, []*model.TaskEntity{taskEntity}); err != nil {
		fmt.Printf("✗ 创建任务失败: %v\n", err)
		return
	}
	fmt.Printf("✓ 任务已创建，ID: %d\n", taskEntity.ID)
	fmt.Println("任务入队成功，等待 Worker 获取并执行...")

	waitForTaskCompletion(ctx, mgr, taskEntity.ID, 30*time.Second, "正常任务")
}

// runTimeoutScenario 测试 2：超时检测和重试
func runTimeoutScenario(ctx context.Context, mgr *manager.Manager) {
	printSection("测试 2: 超时检测和重试")

	fmt.Println("创建一个会超时的任务来测试超时检查机制...")
	fmt.Println("任务配置:")
	fmt.Println("  - 超时时间: 65 秒")
	fmt.Println("  - 实际执行: 70 秒")
	fmt.Println("  - Worker 超时: 10 分钟(禁用 Worker 级超时)")
	fmt.Println("  - 最大重试: 1 次")
	fmt.Println()
	fmt.Println("预期过程:")
	fmt.Println("  1. 任务开始执行（running）")
	fmt.Println("  2. Worker 执行 70 秒")
	fmt.Println("  3. timeoutCheckRoutine 每分钟检查一次超时")
	fmt.Println("  4. 检测到超时后标记为 timeout，重试次数 +1")
	fmt.Println("  5. 根据 CanRetry() 自动重新入队")
	fmt.Println("  6. Worker 重新获取并执行")
	fmt.Println("  7. 重试后仍然超时，最终状态为 timeout（Redo=1/1）")
	fmt.Println()
	fmt.Println("注意：需要等待 timeoutCheckRoutine 检测（约 70 秒）")
	fmt.Println()

	timeoutPayload := DemoPayload{
		Message: "这是一个测试超时的任务",
		UserID:  99999,
	}
	timeoutPayloadJSON, _ := json.Marshal(timeoutPayload)
	timeoutTask := &model.TaskEntity{
		TaskType:    "timeout_task",
		SubjectType: "timeout_test",
		SubjectID:   2,
		Payload:     string(timeoutPayloadJSON),
		Timeout:     65 * time.Second,
		MaxRedo:     1,
		Priority:    0,
		CompanyID:   1,
		Uin:         1001,
	}

	if err := mgr.CreateTasks(ctx, []*model.TaskEntity{timeoutTask}); err != nil {
		fmt.Printf("✗ 创建超时测试任务失败: %v\n", err)
		return
	}
	fmt.Printf("✓ 超时测试任务已创建，ID: %d\n", timeoutTask.ID)

	monitorTaskWithTimeout(ctx, mgr, timeoutTask.ID, 150*time.Second, "超时任务")
}

// runFailureScenario 测试 3：失败任务重试
func runFailureScenario(ctx context.Context, mgr *manager.Manager) {
	printSection("测试 3: 失败任务重试")

	fmt.Println("创建一个会失败的任务来测试重试机制...")
	fmt.Println("任务配置:")
	fmt.Println("  - 执行结果: 返回错误")
	fmt.Println("  - 最大重试: 3 次")
	fmt.Println()
	fmt.Println("预期过程:")
	fmt.Println("  1. 任务执行失败（failed，Redo=1）")
	fmt.Println("  2. 根据 CanRetry() 自动重新入队")
	fmt.Println("  3. Worker 重新获取并执行（第 2 次重试，Redo=2）")
	fmt.Println("  4. 再次失败并重试（第 3 次重试，Redo=3）")
	fmt.Println("  5. 第 3 次重试后仍失败，最终状态为 failed（Redo=3/3）")
	fmt.Println()

	failPayload := DemoPayload{
		Message: "这是一个测试失败的任务",
		UserID:  88888,
	}
	failPayloadJSON, _ := json.Marshal(failPayload)
	failTask := &model.TaskEntity{
		TaskType:    "fail_task",
		SubjectType: "fail_test",
		SubjectID:   3,
		Payload:     string(failPayloadJSON),
		Timeout:     5 * time.Minute,
		MaxRedo:     3,
		Priority:    0,
		CompanyID:   1,
		Uin:         1001,
	}

	if err := mgr.CreateTasks(ctx, []*model.TaskEntity{failTask}); err != nil {
		fmt.Printf("✗ 创建失败测试任务失败: %v\n", err)
		return
	}
	fmt.Printf("✓ 失败测试任务已创建，ID: %d\n", failTask.ID)

	monitorTaskWithTimeout(ctx, mgr, failTask.ID, 60*time.Second, "失败任务")
}

// runHealthCheckScenario 测试 4：健康检查和任务恢复
func runHealthCheckScenario(ctx context.Context, mgr *manager.Manager, w *worker.Worker, healthChecker *health.Checker) {
	printSection("测试 4: 健康检查和任务恢复")

	fmt.Println("模拟 Worker 死亡场景...")
	fmt.Println("配置:")
	fmt.Println("  - 心跳超时时间: 30 秒")
	fmt.Println("  - 健康检查周期: 30 秒")
	fmt.Println("  - 任务超时时间: 5 分钟")
	fmt.Println()
	fmt.Println("预期过程:")
	fmt.Println("  1. 创建任务并启动 Worker")
	fmt.Println("  2. Worker 执行期间定期更新心跳")
	fmt.Println("  3. 执行 10 秒后，停止更新心跳（模拟 Worker 崩溃）")
	fmt.Println("  4. 等待健康检查器检测到心跳超时（约 30 秒）")
	fmt.Println("  5. 触发 OnWorkerDead 回调")
	fmt.Println("  6. 任务被标记为失败并重新入队")
	fmt.Println("  7. Worker 重新获取并执行任务")
	fmt.Println()
	fmt.Println("注意：需要等待约 40 秒才能看到健康检查效果")
	fmt.Println()

	healthPayload := DemoPayload{
		Message: "这是一个测试健康检查的任务",
		UserID:  77777,
	}
	healthPayloadJSON, _ := json.Marshal(healthPayload)
	healthTask := &model.TaskEntity{
		TaskType:    "health_task",
		SubjectType: "health_test",
		SubjectID:   4,
		Payload:     string(healthPayloadJSON),
		Timeout:     5 * time.Minute,
		MaxRedo:     1,
		Priority:    0,
		CompanyID:   1,
		Uin:         1001,
	}

	if err := mgr.CreateTasks(ctx, []*model.TaskEntity{healthTask}); err != nil {
		fmt.Printf("✗ 创建健康检查测试任务失败: %v\n", err)
		return
	}
	fmt.Printf("✓ 健康检查测试任务已创建，ID: %d\n", healthTask.ID)

	monitorTaskWithHealthCheck(ctx, mgr, healthTask.ID, 60*time.Second, "健康检查任务")
}

// waitForTaskCompletion 等待任务完成
func waitForTaskCompletion(ctx context.Context, mgr *manager.Manager, taskID uint, timeout time.Duration, taskName string) {
	fmt.Printf("等待 %s 完成...\n", taskName)
	fmt.Println("========================================")

	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCh:
			fmt.Printf("[DEBUG] 等待 %s 完成超时！\n", taskName)
			return

		case <-ticker.C:
			result, err := mgr.GetTask(ctx, taskID)
			if err != nil {
				fmt.Printf("[DEBUG] 获取任务状态失败: %v\n", err)
				continue
			}

			fmt.Printf("[DEBUG] 任务状态: %s, Redo: %d/%d\n", result.TaskStatus, result.Redo, result.MaxRedo)

			if result.IsFinished() {
				fmt.Println("\n========================================")
				fmt.Printf("%s 执行结果\n", taskName)
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
				return
			}
		}
	}
}

// monitorTaskWithTimeout 监控任务状态变化（带超时）
func monitorTaskWithTimeout(ctx context.Context, mgr *manager.Manager, taskID uint, timeout time.Duration, taskName string) {
	fmt.Printf("监控 %s 状态变化...\n", taskName)

	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	iteration := 0
	var previousStatus model.TaskStatus = ""

	for {
		select {
		case <-timeoutCh:
			fmt.Println("\n========================================")
			fmt.Printf("%s 最终状态\n", taskName)
			fmt.Println("========================================")
			result, err := mgr.GetTask(ctx, taskID)
			if err == nil {
				fmt.Printf("任务 ID: %d\n", result.ID)
				fmt.Printf("任务状态: %s\n", result.TaskStatus)
				fmt.Printf("重试次数: %d/%d\n", result.Redo, result.MaxRedo)
				fmt.Printf("错误信息: %s\n", result.ErrMsg)
				fmt.Printf("执行 Worker: %s\n", result.WorkerID)
			}
			return

		case <-ticker.C:
			iteration++
			result, err := mgr.GetTask(ctx, taskID)
			if err != nil {
				fmt.Printf("[监控第 %d 次] 获取任务状态失败: %v\n", iteration, err)
				continue
			}

			currentStatus := result.TaskStatus

			if previousStatus == "" {
				previousStatus = currentStatus
				fmt.Printf("[监控第 %d 次] 状态: %s, Redo: %d/%d, Worker: %s\n",
					iteration, currentStatus, result.Redo, result.MaxRedo, result.WorkerID)
			} else if currentStatus != previousStatus {
				fmt.Println("")
				fmt.Println("↓ ↓ ↓ ↓ ↓ ↓ ↓ 状态变化 ↓ ↓ ↓ ↓ ↓ ↓ ↓")
				fmt.Printf("[监控第 %d 次] 状态变化: %s → %s\n", iteration, previousStatus, currentStatus)
				fmt.Printf("            Redo: %d/%d, Worker: %s\n", result.Redo, result.MaxRedo, result.WorkerID)

				if previousStatus == model.TaskStatusRunning && currentStatus == model.TaskStatusTimeout {
					fmt.Println("            >>> 检测到超时！任务将被重新入队重试")
				} else if currentStatus == model.TaskStatusPending && previousStatus == model.TaskStatusTimeout {
					fmt.Println("            >>> 超时任务已重新入队，等待 Worker 获取")
				} else if currentStatus == model.TaskStatusRunning && previousStatus == model.TaskStatusPending {
					fmt.Println("            >>> Worker 已获取任务，开始重试执行")
				}
				fmt.Println("↑ ↑ ↑ ↑ ↑ ↑ ↑ 状态变化 ↑ ↑ ↑ ↑ ↑ ↑ ↑")
				previousStatus = currentStatus
			} else {
				fmt.Printf("[监控第 %d 次] 状态: %s, Redo: %d/%d, Worker: %s\n",
					iteration, currentStatus, result.Redo, result.MaxRedo, result.WorkerID)
			}

			if result.IsFinished() && result.Redo >= result.MaxRedo {
				fmt.Println("\n========================================")
				fmt.Printf("%s 最终状态\n", taskName)
				fmt.Println("========================================")
				fmt.Printf("任务 ID: %d\n", result.ID)
				fmt.Printf("任务状态: %s\n", result.TaskStatus)
				fmt.Printf("重试次数: %d/%d\n", result.Redo, result.MaxRedo)
				fmt.Printf("错误信息: %s\n", result.ErrMsg)
				fmt.Printf("执行 Worker: %s\n", result.WorkerID)
				return
			}
		}
	}
}

// monitorTaskWithHealthCheck 监控健康检查任务的执行
func monitorTaskWithHealthCheck(ctx context.Context, mgr *manager.Manager, taskID uint, timeout time.Duration, taskName string) {
	fmt.Printf("监控 %s 执行过程（含健康检查）...\n", taskName)

	timeoutCh := time.After(timeout)
	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	iteration := 0
	for {
		select {
		case <-timeoutCh:
			fmt.Println("\n========================================")
			fmt.Printf("%s 最终状态\n", taskName)
			fmt.Println("========================================")
			result, err := mgr.GetTask(ctx, taskID)
			if err == nil {
				fmt.Printf("任务 ID: %d\n", result.ID)
				fmt.Printf("任务状态: %s\n", result.TaskStatus)
				fmt.Printf("重试次数: %d/%d\n", result.Redo, result.MaxRedo)
				fmt.Printf("错误信息: %s\n", result.ErrMsg)
				fmt.Printf("执行 Worker: %s\n", result.WorkerID)
			}
			fmt.Println("\n健康检查演示已完成")
			return

		case <-ticker.C:
			iteration++
			result, err := mgr.GetTask(ctx, taskID)
			if err != nil {
				fmt.Printf("[监控第 %d 次] 获取任务状态失败: %v\n", iteration, err)
				continue
			}

			fmt.Printf("[监控第 %d 次] 状态: %s, Redo: %d/%d, Worker: %s\n",
				iteration, result.TaskStatus, result.Redo, result.MaxRedo, result.WorkerID)

			if result.IsFinished() && result.Redo >= result.MaxRedo {
				fmt.Println("\n========================================")
				fmt.Printf("%s 最终状态\n", taskName)
				fmt.Println("========================================")
				fmt.Printf("任务 ID: %d\n", result.ID)
				fmt.Printf("任务状态: %s\n", result.TaskStatus)
				fmt.Printf("重试次数: %d/%d\n", result.Redo, result.MaxRedo)
				fmt.Printf("错误信息: %s\n", result.ErrMsg)
				fmt.Printf("执行 Worker: %s\n", result.WorkerID)
				fmt.Println("\n健康检查演示已完成")
				return
			}
		}
	}
}
