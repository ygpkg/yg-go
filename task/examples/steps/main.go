package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/task"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// StepPayload 步骤任务的参数结构
type StepPayload struct {
	StepName    string `json:"step_name"`
	OrderID     int    `json:"order_id"`
	Description string `json:"description"`
}

// StepTaskExecutor 步骤任务执行器
type StepTaskExecutor struct {
	task.BaseExecutor
	payload        StepPayload
	executionOrder *[]int
	mu             *sync.Mutex
}

// OnStart 初始化执行器
func (e *StepTaskExecutor) OnStart(ctx context.Context, taskEntity *task.TaskEntity) error {
	if err := e.BaseExecutor.OnStart(ctx, taskEntity); err != nil {
		return err
	}

	// 解析任务参数
	if err := json.Unmarshal([]byte(taskEntity.Payload), &e.payload); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("\n════════════════════════════════════════\n")
	fmt.Printf("准备执行步骤 %d: %s\n", taskEntity.Step, e.payload.StepName)
	fmt.Printf("════════════════════════════════════════\n")
	fmt.Printf("订单 ID: %d\n", e.payload.OrderID)
	fmt.Printf("AppGroup: %s\n", taskEntity.AppGroup)
	fmt.Printf("描述: %s\n", e.payload.Description)

	return nil
}

// Execute 执行任务
func (e *StepTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("\n→ 执行步骤 %d: %s\n", e.Task.Step, e.payload.StepName)

	// 记录执行顺序
	e.mu.Lock()
	*e.executionOrder = append(*e.executionOrder, e.Task.Step)
	e.mu.Unlock()

	// 模拟步骤处理
	time.Sleep(2 * time.Second)

	fmt.Printf("✓ 步骤 %d 执行完成\n", e.Task.Step)
	return nil
}

// OnSuccess 成功回调
func (e *StepTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("✓ 步骤 %d (%s) 成功\n", e.Task.Step, e.payload.StepName)
	return nil
}

// OnFailure 失败回调
func (e *StepTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("✗ 步骤 %d (%s) 失败\n", e.Task.Step, e.payload.StepName)
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
	fmt.Println("Task 包 - 步骤化任务流程演示")
	fmt.Println("========================================\n")

	// 配置 Worker
	config := &task.TaskConfig{
		WorkerID:          "steps-worker-001",
		MaxConcurrency:    3, // 即使设置多个并发，步骤也会按顺序执行
		Timeout:           10 * time.Minute,
		MaxRedo:           3,
		RedisKeyPrefix:    "task:steps:",
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

	// 记录执行顺序
	var executionOrder []int
	var mu sync.Mutex

	// 注册任务执行器
	worker.RegisterExecutor("validate_order", func() task.TaskExecutor {
		return &StepTaskExecutor{executionOrder: &executionOrder, mu: &mu}
	})
	worker.RegisterExecutor("process_payment", func() task.TaskExecutor {
		return &StepTaskExecutor{executionOrder: &executionOrder, mu: &mu}
	})
	worker.RegisterExecutor("prepare_shipment", func() task.TaskExecutor {
		return &StepTaskExecutor{executionOrder: &executionOrder, mu: &mu}
	})
	worker.RegisterExecutor("send_notification", func() task.TaskExecutor {
		return &StepTaskExecutor{executionOrder: &executionOrder, mu: &mu}
	})
	fmt.Println("✓ 已注册 4 个步骤执行器\n")

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

	// 创建步骤化任务流程
	fmt.Println("========================================")
	fmt.Println("创建订单处理流程")
	fmt.Println("========================================")
	fmt.Println("订单处理包含 4 个步骤:")
	fmt.Println("  步骤 1: 验证订单")
	fmt.Println("  步骤 2: 处理支付")
	fmt.Println("  步骤 3: 准备发货")
	fmt.Println("  步骤 4: 发送通知")
	fmt.Println("\n特点: 只有前一步骤成功，后续步骤才会执行\n")

	orderID := 12345
	appGroup := "order_process_flow" // 相同的 appGroup 将任务组织在一起

	// 创建 4 个步骤的任务
	tasks := []*task.TaskEntity{
		// 步骤 1: 验证订单
		{
			TaskType:    "validate_order",
			SubjectType: "order",
			SubjectID:   uint(orderID),
			AppGroup:    appGroup,
			Step:        1,
			Payload: mustMarshal(StepPayload{
				StepName:    "验证订单",
				OrderID:     orderID,
				Description: "验证订单信息的完整性和有效性",
			}),
			Timeout:   5 * time.Minute,
			MaxRedo:   3,
			Priority:  0,
			CompanyID: 1,
			Uin:       1001,
		},
		// 步骤 2: 处理支付
		{
			TaskType:    "process_payment",
			SubjectType: "order",
			SubjectID:   uint(orderID),
			AppGroup:    appGroup,
			Step:        2,
			Payload: mustMarshal(StepPayload{
				StepName:    "处理支付",
				OrderID:     orderID,
				Description: "调用支付网关处理支付",
			}),
			Timeout:   5 * time.Minute,
			MaxRedo:   3,
			Priority:  0,
			CompanyID: 1,
			Uin:       1001,
		},
		// 步骤 3: 准备发货
		{
			TaskType:    "prepare_shipment",
			SubjectType: "order",
			SubjectID:   uint(orderID),
			AppGroup:    appGroup,
			Step:        3,
			Payload: mustMarshal(StepPayload{
				StepName:    "准备发货",
				OrderID:     orderID,
				Description: "通知仓库准备商品发货",
			}),
			Timeout:   5 * time.Minute,
			MaxRedo:   3,
			Priority:  0,
			CompanyID: 1,
			Uin:       1001,
		},
		// 步骤 4: 发送通知
		{
			TaskType:    "send_notification",
			SubjectType: "order",
			SubjectID:   uint(orderID),
			AppGroup:    appGroup,
			Step:        4,
			Payload: mustMarshal(StepPayload{
				StepName:    "发送通知",
				OrderID:     orderID,
				Description: "向用户发送订单处理成功通知",
			}),
			Timeout:   5 * time.Minute,
			MaxRedo:   3,
			Priority:  0,
			CompanyID: 1,
			Uin:       1001,
		},
	}

	// 批量创建任务
	if err := worker.CreateTasks(ctx, tasks); err != nil {
		fmt.Printf("✗ 创建任务失败: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("✓ 已创建 %d 个步骤任务\n", len(tasks))

	// 监控任务执行
	fmt.Println("\n========================================")
	fmt.Println("监控步骤执行")
	fmt.Println("========================================\n")

	// 等待所有任务完成
	timeout := time.After(60 * time.Second)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	completedSteps := make(map[int]bool)
	lastCompletedCount := 0

	for {
		select {
		case <-timeout:
			fmt.Println("\n✗ 等待任务完成超时")
			return

		case <-ticker.C:
			allCompleted := true
			completedCount := 0

			for _, t := range tasks {
				result, err := worker.GetTask(ctx, t.ID)
				if err != nil {
					fmt.Printf("✗ 获取任务状态失败: %v\n", err)
					return
				}

				if result.IsSuccess() {
					if !completedSteps[result.Step] {
						completedSteps[result.Step] = true
						completedCount++
					}
				} else if !result.IsFinished() {
					allCompleted = false
				}
			}

			// 只在有新步骤完成时输出
			if completedCount > lastCompletedCount {
				fmt.Printf("[监控] 已完成步骤: %d/%d\n", len(completedSteps), len(tasks))
				lastCompletedCount = completedCount
			}

			// 检查是否所有任务都完成
			if allCompleted {
				fmt.Println("\n========================================")
				fmt.Println("流程执行完成")
				fmt.Println("========================================")

				// 显示执行结果
				fmt.Println("\n步骤执行结果:")
				for i, t := range tasks {
					result, _ := worker.GetTask(ctx, t.ID)
					status := "✓"
					if !result.IsSuccess() {
						status = "✗"
					}
					fmt.Printf("  %s 步骤 %d: %s (%s, 耗时: %ds)\n",
						status, i+1, result.TaskType, result.TaskStatus, result.Cost)
				}

				// 验证执行顺序
				mu.Lock()
				order := make([]int, len(executionOrder))
				copy(order, executionOrder)
				mu.Unlock()

				fmt.Println("\n执行顺序验证:")
				fmt.Printf("  实际执行顺序: %v\n", order)
				fmt.Printf("  预期执行顺序: [1 2 3 4]\n")

				isCorrectOrder := len(order) == 4
				for i := 0; i < len(order) && isCorrectOrder; i++ {
					if order[i] != i+1 {
						isCorrectOrder = false
					}
				}

				if isCorrectOrder {
					fmt.Println("  ✓ 步骤按照正确的顺序执行")
				} else {
					fmt.Println("  ✗ 步骤执行顺序不正确")
				}

				fmt.Println("\n========================================")
				fmt.Println("步骤化任务要点")
				fmt.Println("========================================")
				fmt.Println("1. 使用 Step 字段定义步骤顺序")
				fmt.Println("2. 使用 AppGroup 将同一流程的任务组织在一起")
				fmt.Println("3. 相同 SubjectID + AppGroup 的任务按 Step 顺序执行")
				fmt.Println("4. 前置步骤未成功时，后续步骤不会执行")
				fmt.Println("5. 步骤失败后重试不会影响其他步骤")
				fmt.Println("6. 支持同一 SubjectID 的多个流程并行（不同 AppGroup）")

				fmt.Println("\n按 Ctrl+C 退出...")
				quit := make(chan os.Signal, 1)
				signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
				<-quit

				return
			}
		}
	}
}

// mustMarshal JSON 序列化，如果失败则 panic
func mustMarshal(v interface{}) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal: %v", err))
	}
	return string(data)
}
