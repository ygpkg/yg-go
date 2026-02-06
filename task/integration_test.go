//go:build integration
// +build integration

package task

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// 测试执行器：简单的计数任务
type CounterTaskExecutor struct {
	BaseExecutor
	counter *int64
}

func (e *CounterTaskExecutor) Execute(ctx context.Context) error {
	atomic.AddInt64(e.counter, 1)
	time.Sleep(100 * time.Millisecond) // 模拟耗时操作
	return nil
}

// 测试执行器：会失败的任务
type FailingTaskExecutor struct {
	BaseExecutor
	failTimes int
	counter   *int32
}

func (e *FailingTaskExecutor) Prepare(ctx context.Context, task *TaskEntity) error {
	if err := e.BaseExecutor.Prepare(ctx, task); err != nil {
		return err
	}
	
	var payload struct {
		FailTimes int `json:"fail_times"`
	}
	if err := json.Unmarshal([]byte(task.Payload), &payload); err != nil {
		return err
	}
	e.failTimes = payload.FailTimes
	return nil
}

func (e *FailingTaskExecutor) Execute(ctx context.Context) error {
	currentAttempt := atomic.AddInt32(e.counter, 1)
	if int(currentAttempt) <= e.failTimes {
		return fmt.Errorf("task failed on attempt %d", currentAttempt)
	}
	return nil
}

// 测试执行器：超时任务
type TimeoutTaskExecutor struct {
	BaseExecutor
}

func (e *TimeoutTaskExecutor) Execute(ctx context.Context) error {
	// 模拟长时间运行的任务
	select {
	case <-time.After(10 * time.Second):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// 测试执行器：步骤化任务
type StepTaskExecutor struct {
	BaseExecutor
	executionOrder *[]int
	mu             *sync.Mutex
}

func (e *StepTaskExecutor) Execute(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	*e.executionOrder = append(*e.executionOrder, e.Task.Step)
	time.Sleep(50 * time.Millisecond)
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
		Password: "",
		DB:       0,
	})

	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect redis: %w", err)
	}

	return client, nil
}

// TestIntegration_CompleteWorkflow 测试完整的任务执行流程
func TestIntegration_CompleteWorkflow(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	
	// 初始化数据库
	db, err := setupDB()
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}

	// 初始化表
	if err := InitDB(db); err != nil {
		t.Fatalf("Failed to init DB: %v", err)
	}

	// 初始化 Redis
	redisClient, err := setupRedis()
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	// 创建 Worker 配置
	config := &TaskConfig{
		Mode:              ModeDistributed,
		WorkerID:          "test-worker-001",
		MaxConcurrency:    2,
		Timeout:           5 * time.Minute,
		MaxRedo:           3,
		RedisKeyPrefix:    "test:task:",
		EnableHealthCheck: false, // 测试时禁用健康检查
	}

	// 创建 Worker
	worker, err := NewWorker(config, db, redisClient)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	// 注册执行器
	var counter int64
	worker.RegisterExecutor("test_task", func() TaskExecutor {
		return &CounterTaskExecutor{counter: &counter}
	})

	// 启动 Worker
	if err := worker.Start(ctx); err != nil {
		t.Fatalf("Failed to start worker: %v", err)
	}
	defer worker.Stop(ctx)

	// 创建任务
	task := &TaskEntity{
		TaskType:    "test_task",
		SubjectID:   1,
		SubjectType: "test",
		Payload:     `{"test": true}`,
		Timeout:     1 * time.Minute,
		MaxRedo:     3,
		CompanyID:   1,
		Uin:         1001,
	}

	if err := worker.CreateTask(ctx, task); err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// 等待任务完成
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		result, err := worker.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("Failed to get task: %v", err)
		}

		if result.IsFinished() {
			if !result.IsSuccess() {
				t.Fatalf("Task failed: %s", result.ErrMsg)
			}
			// 任务成功完成
			t.Logf("Task completed successfully, counter: %d", atomic.LoadInt64(&counter))
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatal("Task did not complete within timeout")
}

// TestIntegration_TaskRetry 测试任务重试机制
func TestIntegration_TaskRetry(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	db, err := setupDB()
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}

	redisClient, err := setupRedis()
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	config := &TaskConfig{
		Mode:              ModeDistributed,
		WorkerID:          "test-worker-002",
		MaxConcurrency:    1,
		Timeout:           5 * time.Minute,
		MaxRedo:           3,
		RedisKeyPrefix:    "test:task:",
		EnableHealthCheck: false,
	}

	worker, err := NewWorker(config, db, redisClient)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	// 注册会失败的执行器
	var attemptCounter int32
	worker.RegisterExecutor("retry_task", func() TaskExecutor {
		return &FailingTaskExecutor{counter: &attemptCounter}
	})

	if err := worker.Start(ctx); err != nil {
		t.Fatalf("Failed to start worker: %v", err)
	}
	defer worker.Stop(ctx)

	// 创建会失败 2 次然后成功的任务
	payload, _ := json.Marshal(map[string]int{"fail_times": 2})
	task := &TaskEntity{
		TaskType:    "retry_task",
		SubjectID:   2,
		SubjectType: "test",
		Payload:     string(payload),
		Timeout:     1 * time.Minute,
		MaxRedo:     3,
		CompanyID:   1,
		Uin:         1001,
	}

	if err := worker.CreateTask(ctx, task); err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// 等待任务完成（需要重试）
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		result, err := worker.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("Failed to get task: %v", err)
		}

		if result.IsFinished() {
			if !result.IsSuccess() {
				t.Fatalf("Task failed after retries: %s", result.ErrMsg)
			}

			attempts := atomic.LoadInt32(&attemptCounter)
			t.Logf("Task succeeded after %d attempts (redo: %d)", attempts, result.Redo)
			
			if attempts != 3 {
				t.Errorf("Expected 3 attempts, got %d", attempts)
			}
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatal("Task did not complete within timeout")
}

// TestIntegration_TaskTimeout 测试任务超时
func TestIntegration_TaskTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	db, err := setupDB()
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}

	redisClient, err := setupRedis()
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	config := &TaskConfig{
		Mode:              ModeDistributed,
		WorkerID:          "test-worker-003",
		MaxConcurrency:    1,
		Timeout:           5 * time.Minute,
		MaxRedo:           1,
		RedisKeyPrefix:    "test:task:",
		EnableHealthCheck: false,
	}

	worker, err := NewWorker(config, db, redisClient)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	worker.RegisterExecutor("timeout_task", func() TaskExecutor {
		return &TimeoutTaskExecutor{}
	})

	if err := worker.Start(ctx); err != nil {
		t.Fatalf("Failed to start worker: %v", err)
	}
	defer worker.Stop(ctx)

	// 创建一个会超时的任务
	task := &TaskEntity{
		TaskType:    "timeout_task",
		SubjectID:   3,
		SubjectType: "test",
		Payload:     `{}`,
		Timeout:     2 * time.Second, // 很短的超时时间
		MaxRedo:     1,
		CompanyID:   1,
		Uin:         1001,
	}

	if err := worker.CreateTask(ctx, task); err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// 等待任务超时
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		result, err := worker.GetTask(ctx, task.ID)
		if err != nil {
			t.Fatalf("Failed to get task: %v", err)
		}

		if result.TaskStatus == TaskStatusTimeout {
			t.Logf("Task timed out as expected")
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatal("Task did not timeout as expected")
}

// TestIntegration_ConcurrentTasks 测试并发任务执行
func TestIntegration_ConcurrentTasks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	db, err := setupDB()
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}

	redisClient, err := setupRedis()
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	config := &TaskConfig{
		Mode:              ModeDistributed,
		WorkerID:          "test-worker-004",
		MaxConcurrency:    5, // 高并发
		Timeout:           5 * time.Minute,
		MaxRedo:           3,
		RedisKeyPrefix:    "test:task:",
		EnableHealthCheck: false,
	}

	worker, err := NewWorker(config, db, redisClient)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	var counter int64
	worker.RegisterExecutor("concurrent_task", func() TaskExecutor {
		return &CounterTaskExecutor{counter: &counter}
	})

	if err := worker.Start(ctx); err != nil {
		t.Fatalf("Failed to start worker: %v", err)
	}
	defer worker.Stop(ctx)

	// 批量创建任务
	taskCount := 10
	tasks := make([]*TaskEntity, 0, taskCount)
	for i := 0; i < taskCount; i++ {
		task := &TaskEntity{
			TaskType:    "concurrent_task",
			SubjectID:   uint(i + 1),
			SubjectType: "test",
			Payload:     fmt.Sprintf(`{"index": %d}`, i),
			Timeout:     1 * time.Minute,
			MaxRedo:     3,
			CompanyID:   1,
			Uin:         1001,
		}
		tasks = append(tasks, task)
	}

	if err := worker.CreateTasks(ctx, tasks); err != nil {
		t.Fatalf("Failed to create tasks: %v", err)
	}

	// 等待所有任务完成
	deadline := time.Now().Add(30 * time.Second)
	completedCount := 0

	for time.Now().Before(deadline) {
		completedCount = 0
		for _, task := range tasks {
			result, err := worker.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatalf("Failed to get task: %v", err)
			}
			if result.IsSuccess() {
				completedCount++
			}
		}

		if completedCount == taskCount {
			t.Logf("All %d tasks completed, counter: %d", taskCount, atomic.LoadInt64(&counter))
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("Only %d/%d tasks completed within timeout", completedCount, taskCount)
}

// TestIntegration_StepTasks 测试步骤化任务
func TestIntegration_StepTasks(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	db, err := setupDB()
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}

	redisClient, err := setupRedis()
	if err != nil {
		t.Skipf("Redis not available: %v", err)
	}

	config := &TaskConfig{
		Mode:              ModeDistributed,
		WorkerID:          "test-worker-005",
		MaxConcurrency:    1,
		Timeout:           5 * time.Minute,
		MaxRedo:           3,
		RedisKeyPrefix:    "test:task:",
		EnableHealthCheck: false,
	}

	worker, err := NewWorker(config, db, redisClient)
	if err != nil {
		t.Fatalf("Failed to create worker: %v", err)
	}

	var executionOrder []int
	var mu sync.Mutex

	worker.RegisterExecutor("step_task", func() TaskExecutor {
		return &StepTaskExecutor{
			executionOrder: &executionOrder,
			mu:             &mu,
		}
	})

	if err := worker.Start(ctx); err != nil {
		t.Fatalf("Failed to start worker: %v", err)
	}
	defer worker.Stop(ctx)

	// 创建 3 个步骤任务
	subjectID := uint(100)
	appGroup := "test_pipeline"

	tasks := []*TaskEntity{
		{
			TaskType:    "step_task",
			SubjectID:   subjectID,
			SubjectType: "pipeline",
			AppGroup:    appGroup,
			Step:        1,
			Payload:     `{"step": 1}`,
			Timeout:     1 * time.Minute,
			MaxRedo:     3,
			CompanyID:   1,
			Uin:         1001,
		},
		{
			TaskType:    "step_task",
			SubjectID:   subjectID,
			SubjectType: "pipeline",
			AppGroup:    appGroup,
			Step:        2,
			Payload:     `{"step": 2}`,
			Timeout:     1 * time.Minute,
			MaxRedo:     3,
			CompanyID:   1,
			Uin:         1001,
		},
		{
			TaskType:    "step_task",
			SubjectID:   subjectID,
			SubjectType: "pipeline",
			AppGroup:    appGroup,
			Step:        3,
			Payload:     `{"step": 3}`,
			Timeout:     1 * time.Minute,
			MaxRedo:     3,
			CompanyID:   1,
			Uin:         1001,
		},
	}

	if err := worker.CreateTasks(ctx, tasks); err != nil {
		t.Fatalf("Failed to create tasks: %v", err)
	}

	// 等待所有任务完成
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		allCompleted := true
		for _, task := range tasks {
			result, err := worker.GetTask(ctx, task.ID)
			if err != nil {
				t.Fatalf("Failed to get task: %v", err)
			}
			if !result.IsSuccess() {
				allCompleted = false
				break
			}
		}

		if allCompleted {
			// 验证执行顺序
			mu.Lock()
			order := make([]int, len(executionOrder))
			copy(order, executionOrder)
			mu.Unlock()

			t.Logf("Execution order: %v", order)

			// 步骤应该按顺序执行
			if len(order) != 3 {
				t.Errorf("Expected 3 executions, got %d", len(order))
			}
			if len(order) >= 3 && (order[0] != 1 || order[1] != 2 || order[2] != 3) {
				t.Errorf("Tasks executed out of order: %v", order)
			}
			return
		}

		time.Sleep(500 * time.Millisecond)
	}

	t.Fatal("Step tasks did not complete within timeout")
}

// TestIntegration_TaskCancellation 测试任务取消
func TestIntegration_TaskCancellation(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test")
	}

	ctx := context.Background()
	db, err := setupDB()
	if err != nil {
		t.Skipf("Database not available: %v", err)
	}

	// 创建任务但不启动 Worker
	repo := NewTaskRepository(db)

	task := &TaskEntity{
		TaskType:    "cancel_task",
		SubjectID:   999,
		SubjectType: "test",
		Payload:     `{}`,
		Timeout:     1 * time.Minute,
		MaxRedo:     3,
		TaskStatus:  TaskStatusPending,
		CompanyID:   1,
		Uin:         1001,
	}

	if err := repo.CreateTask(ctx, task); err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}

	// 取消任务
	if err := repo.CancelTask(ctx, task.ID, "test cancellation"); err != nil {
		t.Fatalf("Failed to cancel task: %v", err)
	}

	// 验证任务状态
	result, err := repo.GetTaskByID(ctx, task.ID)
	if err != nil {
		t.Fatalf("Failed to get task: %v", err)
	}

	if result.TaskStatus != TaskStatusCanceled {
		t.Errorf("Expected status %s, got %s", TaskStatusCanceled, result.TaskStatus)
	}

	if result.ErrMsg != "test cancellation" {
		t.Errorf("Expected error message 'test cancellation', got '%s'", result.ErrMsg)
	}

	t.Logf("Task cancelled successfully")
}

// BenchmarkTaskCreation 任务创建性能基准测试
func BenchmarkTaskCreation(b *testing.B) {
	ctx := context.Background()
	db, err := setupDB()
	if err != nil {
		b.Skipf("Database not available: %v", err)
	}

	_, err = setupRedis()
	if err != nil {
		b.Skipf("Redis not available: %v", err)
	}

	repo := NewTaskRepository(db)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		task := &TaskEntity{
			TaskType:    "benchmark_task",
			SubjectID:   uint(i + 1),
			SubjectType: "benchmark",
			Payload:     `{"test": true}`,
			Timeout:     1 * time.Minute,
			MaxRedo:     3,
			TaskStatus:  TaskStatusPending,
			CompanyID:   1,
			Uin:         1001,
		}
		repo.CreateTask(ctx, task)
	}
}

// BenchmarkBatchTaskCreation 批量任务创建性能基准测试
func BenchmarkBatchTaskCreation(b *testing.B) {
	ctx := context.Background()
	db, err := setupDB()
	if err != nil {
		b.Skipf("Database not available: %v", err)
	}

	_, err = setupRedis()
	if err != nil {
		b.Skipf("Redis not available: %v", err)
	}

	repo := NewTaskRepository(db)
	batchSize := 100

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		tasks := make([]*TaskEntity, 0, batchSize)
		for j := 0; j < batchSize; j++ {
			task := &TaskEntity{
				TaskType:    "benchmark_task",
				SubjectID:   uint(i*batchSize + j + 1),
				SubjectType: "benchmark",
				Payload:     `{"test": true}`,
				Timeout:     1 * time.Minute,
				MaxRedo:     3,
				TaskStatus:  TaskStatusPending,
				CompanyID:   1,
				Uin:         1001,
			}
			tasks = append(tasks, task)
		}
		repo.CreateTasks(ctx, tasks)
	}
}
