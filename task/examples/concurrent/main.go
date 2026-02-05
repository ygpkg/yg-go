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

	"github.com/ygpkg/yg-go/task"
	"gorm.io/gorm"
)

// ConcurrentPayload 并发任务的参数结构
type ConcurrentPayload struct {
	Index    int    `json:"index"`
	Message  string `json:"message"`
	Duration int    `json:"duration"` // 任务执行时长（毫秒）
}

// ConcurrentTaskExecutor 演示并发处理的任务执行器
type ConcurrentTaskExecutor struct {
	task.BaseExecutor
	payload        ConcurrentPayload
	executingCount *int32 // 当前正在执行的任务数
	completedCount *int32 // 已完成的任务数
	mu             *sync.Mutex
	startTimes     *map[int]time.Time
}

// Prepare 初始化执行器
func (e *ConcurrentTaskExecutor) Prepare(ctx context.Context, taskEntity *task.TaskEntity) error {
	if err := e.BaseExecutor.Prepare(ctx, taskEntity); err != nil {
		return err
	}

	// 解析任务参数
	if err := json.Unmarshal([]byte(taskEntity.Payload), &e.payload); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	// 记录开始时间
	e.mu.Lock()
	(*e.startTimes)[e.payload.Index] = time.Now()
	e.mu.Unlock()

	return nil
}

// Execute 执行任务
func (e *ConcurrentTaskExecutor) Execute(ctx context.Context) error {
	// 增加正在执行的任务计数
	current := atomic.AddInt32(e.executingCount, 1)

	fmt.Printf("[任务 %d] 开始执行 (并发数: %d)\n", e.payload.Index, current)

	// 模拟任务处理
	duration := time.Duration(e.payload.Duration) * time.Millisecond
	time.Sleep(duration)

	// 减少正在执行的任务计数
	atomic.AddInt32(e.executingCount, -1)

	fmt.Printf("[任务 %d] 执行完成 (耗时: %dms)\n", e.payload.Index, e.payload.Duration)

	return nil
}

// OnSuccess 成功回调
func (e *ConcurrentTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	completed := atomic.AddInt32(e.completedCount, 1)

	// 计算执行时长
	e.mu.Lock()
	startTime := (*e.startTimes)[e.payload.Index]
	e.mu.Unlock()
	elapsed := time.Since(startTime)

	fmt.Printf("[任务 %d] ✓ 成功 (总耗时: %v, 已完成: %d)\n",
		e.payload.Index, elapsed.Round(time.Millisecond), completed)

	return nil
}

// OnFailure 失败回调
func (e *ConcurrentTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("[任务 %d] ✗ 失败\n", e.payload.Index)
	return nil
}

func main() {
	fmt.Println("========================================")
	fmt.Println("Task 包 - 并发任务处理演示")
	fmt.Println("========================================\n")

	// 配置 Worker
	maxConcurrency := 5
	config := &task.TaskConfig{
		DBInstance:        "default",
		WorkerID:          "concurrent-worker-001",
		MaxConcurrency:    maxConcurrency, // 并发数
		Timeout:           10 * time.Minute,
		MaxRedo:           3,
		RedisKeyPrefix:    "task:concurrent:",
		EnableHealthCheck: true,
		HealthCheckPeriod: 30 * time.Second,
	}

	fmt.Printf("Worker 配置:\n")
	fmt.Printf("  WorkerID: %s\n", config.WorkerID)
	fmt.Printf("  MaxConcurrency: %d\n", config.MaxConcurrency)
	fmt.Printf("  说明: 最多同时执行 %d 个任务\n\n", maxConcurrency)

	// 创建 Worker
	worker, err := task.NewWorkerWithDBInstance(config)
	if err != nil {
		fmt.Printf("✗ 创建 Worker 失败: %v\n", err)
		fmt.Println("\n提示: 请确保已初始化 dbtools 和 redispool")
		os.Exit(1)
	}
	fmt.Println("✓ Worker 创建成功")

	// 共享状态
	var executingCount int32
	var completedCount int32
	var mu sync.Mutex
	startTimes := make(map[int]time.Time)

	// 注册任务执行器
	worker.RegisterExecutor("concurrent_task", func() task.TaskExecutor {
		return &ConcurrentTaskExecutor{
			executingCount: &executingCount,
			completedCount: &completedCount,
			mu:             &mu,
			startTimes:     &startTimes,
		}
	})
	fmt.Println("✓ 已注册执行器: concurrent_task\n")

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

	// 批量创建任务
	taskCount := 20
	fmt.Println("========================================")
	fmt.Println("创建批量任务")
	fmt.Println("========================================")
	fmt.Printf("创建 %d 个任务，每个任务耗时 500ms\n", taskCount)
	fmt.Printf("理论上串行执行需要: %.1f 秒\n", float64(taskCount)*0.5)
	fmt.Printf("并发执行（并发数=%d）预计: %.1f 秒\n\n",
		maxConcurrency, float64(taskCount)*0.5/float64(maxConcurrency))

	tasks := make([]*task.TaskEntity, 0, taskCount)
	for i := 1; i <= taskCount; i++ {
		payload := ConcurrentPayload{
			Index:    i,
			Message:  fmt.Sprintf("并发任务 %d", i),
			Duration: 500, // 500ms
		}

		payloadJSON, _ := json.Marshal(payload)

		tasks = append(tasks, &task.TaskEntity{
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

	// 批量创建任务
	batchStartTime := time.Now()
	if err := worker.BatchCreateTasks(ctx, tasks); err != nil {
		fmt.Printf("✗ 批量创建任务失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ 已创建 %d 个任务\n\n", len(tasks))

	// 监控任务执行
	fmt.Println("========================================")
	fmt.Println("监控任务执行")
	fmt.Println("========================================\n")

	// 实时统计
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			<-ticker.C
			executing := atomic.LoadInt32(&executingCount)
			completed := atomic.LoadInt32(&completedCount)

			if completed >= int32(taskCount) {
				return
			}

			fmt.Printf("[统计] 正在执行: %d | 已完成: %d/%d\n",
				executing, completed, taskCount)
		}
	}()

	// 等待所有任务完成
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			fmt.Println("\n✗ 等待任务完成超时")
			return

		case <-ticker.C:
			completed := atomic.LoadInt32(&completedCount)
			if completed >= int32(taskCount) {
				// 所有任务完成
				totalTime := time.Since(batchStartTime)

				fmt.Println("\n========================================")
				fmt.Println("任务执行完成")
				fmt.Println("========================================")
				fmt.Printf("任务总数: %d\n", taskCount)
				fmt.Printf("总耗时: %v\n", totalTime.Round(time.Millisecond))
				fmt.Printf("平均每个任务: %v\n",
					(totalTime / time.Duration(taskCount)).Round(time.Millisecond))

				// 计算加速比
				serialTime := float64(taskCount) * 0.5
				speedup := serialTime / totalTime.Seconds()
				efficiency := speedup / float64(maxConcurrency) * 100

				fmt.Printf("\n性能分析:\n")
				fmt.Printf("  串行执行预计: %.1f 秒\n", serialTime)
				fmt.Printf("  实际耗时: %.1f 秒\n", totalTime.Seconds())
				fmt.Printf("  加速比: %.2fx\n", speedup)
				fmt.Printf("  并发效率: %.1f%%\n", efficiency)

				// 验证所有任务状态
				fmt.Println("\n验证任务状态:")
				successCount := 0
				for _, t := range tasks {
					result, err := worker.GetTask(ctx, t.ID)
					if err != nil {
						fmt.Printf("  ✗ 获取任务 %d 状态失败: %v\n", t.ID, err)
						continue
					}
					if result.IsSuccess() {
						successCount++
					}
				}
				fmt.Printf("  成功: %d/%d\n", successCount, taskCount)

				fmt.Println("\n========================================")
				fmt.Println("并发处理要点")
				fmt.Println("========================================")
				fmt.Println("1. MaxConcurrency 控制同时执行的任务数")
				fmt.Println("2. 批量创建任务可以提高效率")
				fmt.Println("3. Worker 自动负载均衡任务分发")
				fmt.Println("4. 每个任务执行器实例是独立的")
				fmt.Println("5. 合理设置并发数可以充分利用资源")
				fmt.Println("6. 过高的并发数可能导致资源竞争")

				fmt.Println("\n按 Ctrl+C 退出...")
				quit := make(chan os.Signal, 1)
				signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
				<-quit

				return
			}
		}
	}
}
