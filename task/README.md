# Task - 分布式任务队列

`task` 是一个基于 Redis Stream 的分布式任务队列系统，提供可靠的任务调度和执行能力。

## 特性

- **分布式架构**: 基于 Redis Stream 实现分布式任务队列，支持多 Worker 并发处理
- **任务状态管理**: 支持 pending、running、success、failed、canceled、timeout 等状态
- **自动重试**: 任务失败后自动重试，支持配置最大重试次数
- **超时控制**: 任务执行超时自动标记并支持重试
- **步骤化执行**: 支持多步骤任务流程，前置步骤未完成时后续步骤不执行
- **优先级调度**: 支持任务优先级，高优先级任务优先执行
- **父子任务**: 支持父子任务关系，便于构建复杂业务流程
- **心跳检查**: Worker 定期上报心跳，自动检测故障并恢复任务
- **并发控制**: 可配置每个 Worker 的最大并发数
- **多租户支持**: 支持 CompanyID 和 Uin 字段，适用于多租户场景

## 安装

```bash
go get github.com/ygpkg/yg-go/task
```

## 核心概念

### TaskEntity 结构

任务实体包含以下主要字段：

```go
type TaskEntity struct {
    gorm.Model
    CompanyID   uint          // 公司 ID（多租户）
    Uin         uint          // 用户 ID
    SubjectType string        // 主体类型（如：order, document, user）
    SubjectID   uint          // 主体 ID（业务对象 ID）
    TaskType    string        // 任务类型（用于匹配执行器）
    TaskStatus  TaskStatus    // 任务状态
    Priority    int           // 优先级（数值越大优先级越高）
    Step        int           // 步骤序号（步骤化任务）
    Redo        int           // 当前重试次数
    MaxRedo     int           // 最大重试次数
    Timeout     time.Duration // 超时时间
    Payload     string        // 任务参数（JSON）
    Result      string        // 执行结果（JSON）
    ErrMsg      string        // 错误信息
    WorkerID    string        // Worker 标识
    StartAt     *time.Time    // 开始时间
    EndAt       *time.Time    // 结束时间
    Cost        int64         // 执行耗时（秒）
    ParentID    uint          // 父任务 ID
    AppGroup    string        // 应用分组（用于步骤化任务）
}
```

### Worker 架构

Worker 是任务的执行者，包含以下核心组件：

- **队列（Queue）**: 基于 Redis Stream 实现，支持消息分发和消费组
- **执行器注册表（ExecutorRegistry）**: 管理任务类型和执行器的映射
- **健康检查器（HealthChecker）**: 监控 Worker 心跳，自动恢复故障任务
- **任务仓库（TaskRepository）**: 数据库访问层，管理任务持久化

### 任务执行器

任务执行器需要实现 `TaskExecutor` 接口：

```go
type TaskExecutor interface {
    Prepare(ctx context.Context, task *TaskEntity) error
    Execute(ctx context.Context) error
    OnSuccess(ctx context.Context, tx *gorm.DB) error
    OnFailure(ctx context.Context, tx *gorm.DB) error
}
```

## 快速开始

### 1. 初始化数据库表

```go
import (
    "github.com/ygpkg/yg-go/task"
    "gorm.io/gorm"
)

func InitDB(db *gorm.DB) error {
    return task.Init(db)
}
```

### 2. 定义任务执行器

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	
	"github.com/ygpkg/yg-go/task"
	"gorm.io/gorm"
)

// MyTaskPayload 任务参数结构
type MyTaskPayload struct {
	Message string `json:"message"`
	Count   int    `json:"count"`
}

// MyTaskExecutor 自定义任务执行器
type MyTaskExecutor struct {
	task.BaseExecutor
	payload MyTaskPayload
}

// Prepare 初始化执行器
func (e *MyTaskExecutor) Prepare(ctx context.Context, taskEntity *task.TaskEntity) error {
	// 调用基类 Prepare
	if err := e.BaseExecutor.Prepare(ctx, taskEntity); err != nil {
		return err
	}
	
	// 解析任务参数
	if err := json.Unmarshal([]byte(taskEntity.Payload), &e.payload); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}
	
	return nil
}

// Execute 执行任务
func (e *MyTaskExecutor) Execute(ctx context.Context) error {
	// 执行具体的业务逻辑
	fmt.Printf("Processing task: %s (count: %d)\n", e.payload.Message, e.payload.Count)
	
	// 你的业务逻辑...
	
	return nil
}

// OnSuccess 成功回调（可选）
func (e *MyTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	// 任务成功后的操作，如更新数据库
	// tx 是一个数据库事务，可以在这里进行事务性操作
	return nil
}

// OnFailure 失败回调（可选）
func (e *MyTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	// 任务失败后的操作，如记录日志
	return nil
}
```

### 3. 创建和启动 Worker

```go
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
	
	"github.com/ygpkg/yg-go/task"
)

func main() {
	// 配置 Worker
	config := &task.TaskConfig{
		WorkerID:          "worker-001", // Worker 唯一标识
		MaxConcurrency:    5,            // 最大并发数
		Timeout:           10 * time.Minute, // 默认超时
		MaxRedo:           3,            // 默认最大重试次数
		RedisKeyPrefix:    "task:",      // Redis 键前缀
		EnableHealthCheck: true,         // 启用健康检查
		HealthCheckPeriod: 30 * time.Second, // 健康检查周期
	}
	
	// 获取数据库实例
	db := dbtools.DB("default")
	if db == nil {
		panic(fmt.Sprintf("Database instance not found: %s", "default"))
	}
	
	// 创建 Redis 客户端
	redisClient := redis.NewClient(&redis.Options{
		Addr: "localhost:6379",
	})
	
	// 创建 Worker
	worker, err := task.NewWorker(config, db, redisClient)
	if err != nil {
		panic(fmt.Sprintf("Failed to create worker: %v", err))
	}
	
	// 注册任务执行器
	worker.RegisterExecutor("my_task_type", func() task.TaskExecutor {
		return &MyTaskExecutor{}
	})
	
	// 启动 Worker
	ctx := context.Background()
	if err := worker.Start(ctx); err != nil {
		panic(fmt.Sprintf("Failed to start worker: %v", err))
	}
	
	fmt.Println("Worker started successfully")
	
	// 等待退出信号
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	
	fmt.Println("Shutting down worker...")
	
	// 优雅关闭
	if err := worker.Stop(ctx); err != nil {
		fmt.Printf("Error stopping worker: %v\n", err)
	}
	
	fmt.Println("Worker stopped")
}
```

### 4. 创建任务

```go
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
	
	"github.com/ygpkg/yg-go/task"
)

func CreateTask(worker *task.Worker) error {
	// 准备任务参数
	payload := MyTaskPayload{
		Message: "Hello, Task!",
		Count:   42,
	}
	
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}
	
	// 创建任务
	taskEntity := &task.TaskEntity{
		TaskType:    "my_task_type",
		SubjectType: "order",
		SubjectID:   12345,
		Payload:     string(payloadJSON),
		Timeout:     5 * time.Minute,
		MaxRedo:     3,
		Priority:    0,
		CompanyID:   1,
		Uin:         1001,
	}
	
	ctx := context.Background()
	if err := worker.CreateTask(ctx, taskEntity); err != nil {
		return fmt.Errorf("failed to create task: %w", err)
	}
	
	fmt.Printf("Task created with ID: %d\n", taskEntity.ID)
	return nil
}
```

## 高级用法

### 步骤化任务

步骤化任务允许你创建多个有依赖关系的任务步骤。只有前置步骤完成后，后续步骤才会被执行。

```go
import (
	"context"
	"time"
	
	"github.com/ygpkg/yg-go/task"
)

func CreateStepTasks(worker *task.Worker) error {
	ctx := context.Background()
	subjectID := uint(100)
	appGroup := "order_process" // 相同的 appGroup 将任务组织在一起
	
	// 创建多个步骤的任务
	tasks := []*task.TaskEntity{
		{
			TaskType:    "validate_order",
			SubjectType: "order",
			SubjectID:   subjectID,
			AppGroup:    appGroup,
			Step:        1, // 步骤 1：验证订单
			Payload:     `{"action": "validate"}`,
			Timeout:     5 * time.Minute,
			MaxRedo:     3,
			CompanyID:   1,
			Uin:         1001,
		},
		{
			TaskType:    "process_payment",
			SubjectType: "order",
			SubjectID:   subjectID,
			AppGroup:    appGroup,
			Step:        2, // 步骤 2：处理支付（等待步骤 1 完成）
			Payload:     `{"action": "payment"}`,
			Timeout:     5 * time.Minute,
			MaxRedo:     3,
			CompanyID:   1,
			Uin:         1001,
		},
		{
			TaskType:    "send_notification",
			SubjectType: "order",
			SubjectID:   subjectID,
			AppGroup:    appGroup,
			Step:        3, // 步骤 3：发送通知（等待步骤 2 完成）
			Payload:     `{"action": "notify"}`,
			Timeout:     5 * time.Minute,
			MaxRedo:     3,
			CompanyID:   1,
			Uin:         1001,
		},
	}
	
	// 批量创建任务
	if err := worker.CreateTasks(ctx, tasks); err != nil {
		return err
	}
	
	return nil
}
```

### 父子任务

父子任务关系用于构建任务树结构，便于追踪和管理复杂的任务依赖。

```go
func CreateParentChildTasks(worker *task.Worker) error {
	ctx := context.Background()
	
	// 创建父任务
	parentTask := &task.TaskEntity{
		TaskType:    "export_data",
		SubjectType: "export",
		SubjectID:   999,
		Payload:     `{"format": "csv"}`,
		Timeout:     30 * time.Minute,
		MaxRedo:     2,
		CompanyID:   1,
		Uin:         1001,
	}
	
	if err := worker.CreateTask(ctx, parentTask); err != nil {
		return err
	}
	
	// 创建子任务
	childTasks := []*task.TaskEntity{
		{
			TaskType:    "export_chunk",
			SubjectType: "export",
			SubjectID:   999,
			ParentID:    parentTask.ID, // 关联父任务
			Payload:     `{"chunk": 1, "offset": 0, "limit": 1000}`,
			Timeout:     10 * time.Minute,
			MaxRedo:     3,
			CompanyID:   1,
			Uin:         1001,
		},
		{
			TaskType:    "export_chunk",
			SubjectType: "export",
			SubjectID:   999,
			ParentID:    parentTask.ID, // 关联父任务
			Payload:     `{"chunk": 2, "offset": 1000, "limit": 1000}`,
			Timeout:     10 * time.Minute,
			MaxRedo:     3,
			CompanyID:   1,
			Uin:         1001,
		},
	}
	
	if err := worker.CreateTasks(ctx, childTasks); err != nil {
		return err
	}
	
	return nil
}
```

### 批量创建任务

批量创建任务可以提高性能，减少数据库往返次数。

```go
func CreateBatchTasks(worker *task.Worker) error {
	ctx := context.Background()
	
	// 创建 100 个任务
	tasks := make([]*task.TaskEntity, 0, 100)
	for i := 0; i < 100; i++ {
		tasks = append(tasks, &task.TaskEntity{
			TaskType:    "batch_task",
			SubjectType: "batch",
			SubjectID:   uint(i + 1),
			Payload:     fmt.Sprintf(`{"index": %d}`, i),
			Timeout:     5 * time.Minute,
			MaxRedo:     3,
			CompanyID:   1,
			Uin:         1001,
		})
	}
	
	// 批量创建
	if err := worker.CreateTasks(ctx, tasks); err != nil {
		return err
	}
	
	fmt.Printf("Created %d tasks\n", len(tasks))
	return nil
}
```

### 查询任务状态

```go
func QueryTaskStatus(worker *task.Worker, taskID uint) error {
	ctx := context.Background()
	
	// 获取任务信息
	taskEntity, err := worker.GetTask(ctx, taskID)
	if err != nil {
		return err
	}
	
	fmt.Printf("任务 ID: %d\n", taskEntity.ID)
	fmt.Printf("任务类型: %s\n", taskEntity.TaskType)
	fmt.Printf("任务状态: %s\n", taskEntity.TaskStatus)
	fmt.Printf("重试次数: %d/%d\n", taskEntity.Redo, taskEntity.MaxRedo)
	
	if taskEntity.IsSuccess() {
		fmt.Printf("执行结果: %s\n", taskEntity.Result)
		fmt.Printf("执行耗时: %d 秒\n", taskEntity.Cost)
	} else if taskEntity.TaskStatus == task.TaskStatusFailed || 
	          taskEntity.TaskStatus == task.TaskStatusTimeout {
		fmt.Printf("错误信息: %s\n", taskEntity.ErrMsg)
	}
	
	return nil
}
```

### 取消任务

取消任务只能取消处于 pending 或 failed 状态的任务。

```go
func CancelTask(worker *task.Worker, taskID uint) error {
	ctx := context.Background()
	
	// 取消任务
	if err := worker.CancelTask(ctx, taskID, "用户主动取消"); err != nil {
		return err
	}
	
	fmt.Printf("任务 %d 已取消\n", taskID)
	return nil
}
```

## 配置说明

### TaskConfig 字段

| 字段 | 类型 | 说明 | 默认值 | 必填 |
|------|------|------|--------|------|
| WorkerID | string | Worker 唯一标识 | "" | 是 |
| MaxConcurrency | int | 每个任务类型的最大并发数 | 5 | 否 |
| Timeout | time.Duration | 任务默认超时时间 | 10分钟 | 否 |
| MaxRedo | int | 任务默认最大重试次数 | 3 | 否 |
| RedisKeyPrefix | string | Redis 键前缀 | "task:" | 否 |
| EnableHealthCheck | bool | 是否启用健康检查 | true | 否 |
| HealthCheckPeriod | time.Duration | 健康检查周期 | 30秒 | 否 |

### 配置示例

```go
config := &task.TaskConfig{
	WorkerID:          "worker-prod-001", // Worker ID（建议包含环境和编号）
	MaxConcurrency:    10,                // 并发数（根据机器性能调整）
	Timeout:           15 * time.Minute,  // 默认超时
	MaxRedo:           3,                 // 最大重试次数
	RedisKeyPrefix:    "prod:task:",      // 生产环境前缀
	EnableHealthCheck: true,              // 启用健康检查
	HealthCheckPeriod: 30 * time.Second,  // 每 30 秒检查一次
}
```

## 数据库表结构

任务数据存储在 `core_task` 表中：

```sql
CREATE TABLE `core_task` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT COMMENT '主键ID',
  `created_at` datetime(3) DEFAULT NULL COMMENT '创建时间',
  `updated_at` datetime(3) DEFAULT NULL COMMENT '更新时间',
  `deleted_at` datetime(3) DEFAULT NULL COMMENT '删除时间',
  `company_id` bigint NOT NULL DEFAULT '0' COMMENT '公司ID（多租户）',
  `uin` bigint NOT NULL DEFAULT '0' COMMENT '用户ID',
  `subject_type` varchar(64) NOT NULL COMMENT '主体类型（如：order, document）',
  `subject_id` bigint NOT NULL COMMENT '主体ID（业务对象ID）',
  `task_type` varchar(64) NOT NULL COMMENT '任务类型（匹配执行器）',
  `task_status` varchar(20) NOT NULL COMMENT '任务状态',
  `priority` int NOT NULL DEFAULT '0' COMMENT '优先级（越大越高）',
  `step` int NOT NULL DEFAULT '0' COMMENT '步骤序号',
  `redo` int NOT NULL DEFAULT '0' COMMENT '当前重试次数',
  `max_redo` int NOT NULL DEFAULT '3' COMMENT '最大重试次数',
  `timeout` bigint NOT NULL COMMENT '超时时间（纳秒）',
  `payload` text COMMENT '任务参数（JSON）',
  `result` text COMMENT '执行结果（JSON）',
  `err_msg` text COMMENT '错误信息',
  `worker_id` varchar(64) DEFAULT NULL COMMENT 'Worker标识',
  `start_at` datetime DEFAULT NULL COMMENT '开始执行时间',
  `end_at` datetime DEFAULT NULL COMMENT '结束时间',
  `cost` bigint DEFAULT '0' COMMENT '执行耗时（秒）',
  `parent_id` bigint DEFAULT '0' COMMENT '父任务ID',
  `app_group` varchar(32) DEFAULT NULL COMMENT '应用分组（步骤化任务）',
  PRIMARY KEY (`id`),
  KEY `idx_task_type` (`task_type`),
  KEY `idx_task_status` (`task_status`),
  KEY `idx_subject_type` (`subject_type`),
  KEY `idx_subject_id` (`subject_id`),
  KEY `idx_priority` (`priority`),
  KEY `idx_parent_id` (`parent_id`),
  KEY `idx_company_id` (`company_id`),
  KEY `idx_uin` (`uin`),
  KEY `idx_app_group` (`app_group`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COMMENT='任务队列表';
```

### 字段说明

| 字段 | 说明 | 用途 |
|------|------|------|
| company_id | 公司 ID | 多租户隔离，0 表示不区分公司 |
| uin | 用户 ID | 标识任务归属用户，0 表示系统任务 |
| subject_type | 主体类型 | 业务对象类型（如：order, document, export） |
| subject_id | 主体 ID | 业务对象的唯一标识 |
| task_type | 任务类型 | 用于匹配执行器，必须在 Worker 中注册 |
| priority | 优先级 | 数值越大优先级越高，相同优先级按创建时间排序 |
| step | 步骤序号 | 步骤化任务的序号，前置步骤未完成时后续步骤不执行 |
| app_group | 应用分组 | 将同一业务流程的任务组织在一起，支持同一 subject_id 的多个流程并行 |
| parent_id | 父任务 ID | 构建父子任务关系，0 表示无父任务 |

### 初始化数据库表

```go
import (
	"github.com/ygpkg/yg-go/task"
	"gorm.io/gorm"
)

func InitDB(db *gorm.DB) error {
	return task.Init(db)
}
```

## 最佳实践

### 1. 任务执行器设计

**保持无状态**
```go
// ✅ 好的做法：使用 BaseExecutor，状态存储在实例中
type GoodExecutor struct {
	task.BaseExecutor
	config MyConfig // 从 Prepare 中初始化
}

// ❌ 避免：使用全局变量或包级变量
var globalConfig MyConfig // 不推荐
```

**参数序列化**
```go
// ✅ 定义明确的参数结构
type TaskPayload struct {
	UserID   int    `json:"user_id"`
	Action   string `json:"action"`
	Metadata map[string]interface{} `json:"metadata"`
}

func (e *MyExecutor) Prepare(ctx context.Context, t *task.TaskEntity) error {
	if err := e.BaseExecutor.Prepare(ctx, t); err != nil {
		return err
	}
	
	var payload TaskPayload
	if err := json.Unmarshal([]byte(t.Payload), &payload); err != nil {
		return fmt.Errorf("invalid payload: %w", err)
	}
	
	// 验证参数
	if payload.UserID <= 0 {
		return fmt.Errorf("invalid user_id: %d", payload.UserID)
	}
	
	e.payload = payload
	return nil
}
```

**事务性操作**
```go
// ✅ 在回调中使用事务
func (e *MyExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	// 更新关联数据，与任务状态更新在同一事务中
	return tx.Model(&Order{}).
		Where("id = ?", e.payload.OrderID).
		Update("status", "processed").
		Error
}
```

### 2. 错误处理

**明确的错误信息**
```go
func (e *MyExecutor) Execute(ctx context.Context) error {
	// ✅ 包含上下文的错误信息
	if err := e.processOrder(); err != nil {
		return fmt.Errorf("failed to process order %d: %w", e.orderID, err)
	}
	
	// ❌ 模糊的错误信息
	// return errors.New("process failed")
	
	return nil
}
```

**区分错误类型**
```go
// 对于不可重试的错误，可以将任务标记为取消
func (e *MyExecutor) Execute(ctx context.Context) error {
	user, err := e.getUser()
	if err == ErrUserNotFound {
		// 用户不存在，不需要重试
		// 可以在 OnFailure 中判断错误类型并取消任务
		return fmt.Errorf("user not found, cannot retry: %w", err)
	}
	
	// 其他错误可以重试
	return err
}
```

### 3. 超时设置

**根据任务特性设置超时**
```go
// 快速任务：30秒 - 2分钟
quickTask := &task.TaskEntity{
	TaskType: "send_email",
	Timeout:  1 * time.Minute,
}

// 中等任务：5 - 15分钟
mediumTask := &task.TaskEntity{
	TaskType: "generate_report",
	Timeout:  10 * time.Minute,
}

// 长时间任务：30分钟 - 2小时
longTask := &task.TaskEntity{
	TaskType: "export_large_dataset",
	Timeout:  1 * time.Hour,
}
```

**检查上下文取消**
```go
func (e *MyExecutor) Execute(ctx context.Context) error {
	for i := 0; i < 1000; i++ {
		// 定期检查上下文
		select {
		case <-ctx.Done():
			return ctx.Err() // 优雅退出
		default:
		}
		
		// 处理单个项目
		if err := e.processItem(i); err != nil {
			return err
		}
	}
	return nil
}
```

### 4. 并发控制

**根据任务类型调整并发数**
```go
// CPU 密集型任务：并发数 = CPU 核心数
cpuBoundConfig := &task.TaskConfig{
	WorkerID:       "worker-cpu-001",
	MaxConcurrency: runtime.NumCPU(),
}

// I/O 密集型任务：并发数可以更高
ioBoundConfig := &task.TaskConfig{
	WorkerID:       "worker-io-001",
	MaxConcurrency: runtime.NumCPU() * 4,
}

// 外部 API 调用：根据 API 限流设置
apiCallConfig := &task.TaskConfig{
	WorkerID:       "worker-api-001",
	MaxConcurrency: 10, // 假设 API 限制每秒 10 个请求
}
```

### 5. Worker 部署

**生产环境建议**
```go
// 为不同任务类型部署专用 Worker
func DeployWorkers() {
	// Worker 1: 处理快速任务
	quickWorker := createWorker("quick-worker-001", 20)
	quickWorker.RegisterExecutor("send_email", ...)
	quickWorker.RegisterExecutor("send_sms", ...)
	
	// Worker 2: 处理慢任务
	slowWorker := createWorker("slow-worker-001", 5)
	slowWorker.RegisterExecutor("export_data", ...)
	slowWorker.RegisterExecutor("generate_report", ...)
	
	// Worker 3: 处理高优先级任务
	priorityWorker := createWorker("priority-worker-001", 10)
	priorityWorker.RegisterExecutor("urgent_notification", ...)
}
```

**水平扩展**
```go
// 启动多个 Worker 实例处理同一任务类型
// 在不同机器上运行，使用不同的 WorkerID
// Worker A (机器 1)
workerA := createWorker("worker-a-001", 10)
workerA.RegisterExecutor("process_order", ...)

// Worker B (机器 2)
workerB := createWorker("worker-b-001", 10)
workerB.RegisterExecutor("process_order", ...)

// 两个 Worker 会自动负载均衡，处理同一队列的任务
```

## 监控和运维

### 任务统计查询

```go
import (
	"context"
	"github.com/ygpkg/yg-go/task"
)

// 获取待处理任务数量
func GetPendingTaskCount(repo *task.TaskRepository, taskType string) (int64, error) {
	ctx := context.Background()
	count, err := repo.GetPendingTaskCount(ctx, taskType)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// 获取任务执行统计
func GetTaskStats(dao *task.TaskDao) error {
	ctx := context.Background()
	
	// 各状态任务数量
	successCount, _ := dao.CountByCond(ctx, &task.TaskCond{
		TaskStatus: task.TaskStatusSuccess,
	})
	
	failedCount, _ := dao.CountByCond(ctx, &task.TaskCond{
		TaskStatus: task.TaskStatusFailed,
	})
	
	// 平均执行时间
	avgCost, _ := dao.AvgCostTimeByCond(ctx, &task.TaskCond{
		TaskStatus: task.TaskStatusSuccess,
	})
	
	fmt.Printf("成功: %d, 失败: %d, 平均耗时: %.2f秒\n", 
		successCount, failedCount, avgCost)
	
	return nil
}
```

### 健康检查

Worker 会自动进行健康检查：

- **心跳监控**: Worker 每 5 秒上报一次心跳
- **超时检测**: 30 秒未收到心跳的 Worker 被标记为失败
- **任务恢复**: 失败 Worker 的任务自动重新调度
- **队列同步**: 定期检查队列消息数量，自动补充

### 日志

任务队列使用 `yg-go/logs` 包记录日志：

```go
// 日志级别和内容
// INFO: 任务创建、启动、完成
[task] created task, id: 123, type: process_order
[task] worker started, workerID: worker-001
[task] task success

// WARN: 任务超时、Worker 过期
[task] task timeout
[task] worker expired: worker-002, taskType: export_data

// ERROR: 任务失败、系统错误
[task] task failed: database connection error
[task] failed to pull task: redis connection refused

// DEBUG: 详细执行信息（开发环境）
[task] push task to queue, taskType: send_email, msgID: 1234567890
[task] pop task from queue, taskType: send_email, workerID: worker-001
```

### 性能监控指标

建议监控以下指标：

| 指标 | 说明 | 告警阈值建议 |
|------|------|-------------|
| 待处理任务数 | pending + failed 状态的任务数 | > 10000 |
| 任务失败率 | failed / total | > 5% |
| 任务平均耗时 | 成功任务的平均执行时间 | 根据任务类型设定 |
| Worker 数量 | 当前活跃的 Worker 数量 | < 期望值 |
| 任务积压时间 | 任务创建到执行的时间差 | > 10分钟 |

## API 参考

### Worker 接口

```go
// 创建 Worker
func NewWorker(config *TaskConfig, db *gorm.DB) (*Worker, error)

// 注册执行器
func (w *Worker) RegisterExecutor(taskType string, factory ExecutorFactory)

// 生命周期管理
func (w *Worker) Start(ctx context.Context) error
func (w *Worker) Stop(ctx context.Context) error

// 任务操作
func (w *Worker) CreateTask(ctx context.Context, task *TaskEntity) error
func (w *Worker) CreateTasks(ctx context.Context, tasks []*TaskEntity) error
func (w *Worker) GetTask(ctx context.Context, taskID uint) (*TaskEntity, error)
func (w *Worker) CancelTask(ctx context.Context, taskID uint, reason string) error
```

### TaskExecutor 接口

```go
type TaskExecutor interface {
    // 初始化执行器
    Prepare(ctx context.Context, task *TaskEntity) error
    
    // 执行任务
    Execute(ctx context.Context) error
    
    // 成功回调（在事务中执行）
    OnSuccess(ctx context.Context, tx *gorm.DB) error
    
    // 失败回调（在事务中执行）
    OnFailure(ctx context.Context, tx *gorm.DB) error
}
```

### TaskEntity 方法

```go
// 状态判断
func (t *TaskEntity) IsPending() bool
func (t *TaskEntity) IsRunning() bool
func (t *TaskEntity) IsFinished() bool
func (t *TaskEntity) IsSuccess() bool
func (t *TaskEntity) CanRetry() bool

// 状态更新
func (t *TaskEntity) MarkAsRunning(workerID string)
func (t *TaskEntity) MarkAsSuccess(result string)
func (t *TaskEntity) MarkAsFailed(errMsg string)
func (t *TaskEntity) MarkAsTimeout()
func (t *TaskEntity) MarkAsCanceled(reason string)

// 验证
func (t *TaskEntity) Validate() error
```

## 常见问题

### Q: 任务重试机制如何工作？

**A**: 当任务执行失败或超时时，系统会自动增加 `Redo` 计数并将任务重新推入队列。只要 `Redo < MaxRedo`，任务就会被重新调度执行。

```go
// 任务失败后
task.Redo = 1  // 增加重试计数
task.TaskStatus = TaskStatusFailed
// 如果 Redo < MaxRedo，任务会被重新推入队列
```

### Q: 如何确保任务不会被重复执行？

**A**: 系统使用多重机制防止任务重复执行：

1. **Redis Stream Consumer Group**: 确保每条消息只被一个 Worker 消费
2. **数据库行锁**: 使用 `SELECT ... FOR UPDATE SKIP LOCKED` 确保并发安全
3. **状态机**: 任务状态只能按特定流程转换（pending → running → success/failed）

### Q: 任务超时后会发生什么？

**A**: 任务超时后：
1. 任务状态更新为 `timeout`
2. `Redo` 计数加 1
3. 如果 `Redo < MaxRedo`，任务重新推入队列
4. 如果达到最大重试次数，任务不再执行

### Q: 如何处理长时间运行的任务？

**A**: 对于长时间运行的任务：

```go
// 1. 设置足够长的超时时间
task.Timeout = 2 * time.Hour

// 2. 在执行过程中定期检查上下文
func (e *MyExecutor) Execute(ctx context.Context) error {
    for i := 0; i < largeDataset; i++ {
        // 检查是否被取消
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
        
        // 处理数据...
    }
    return nil
}

// 3. 考虑拆分为多个小任务
```

### Q: 步骤化任务如何确保顺序执行？

**A**: 系统在获取待处理任务时会检查：
- 相同 `SubjectID` 和 `AppGroup` 的前置步骤是否完成
- 只有前置步骤（较小的 `Step` 值）全部成功，当前步骤才会执行

```sql
-- 获取任务时排除前置步骤未完成的任务
WHERE NOT EXISTS (
    SELECT 1 FROM core_task t2
    WHERE t2.subject_id = core_task.subject_id
      AND t2.app_group = core_task.app_group
      AND t2.step < core_task.step
      AND t2.task_status NOT IN ('canceled', 'success')
)
```

### Q: Worker 宕机后任务会丢失吗？

**A**: 不会。Worker 宕机后：
1. 心跳检查器检测到 Worker 超时（30秒）
2. 正在执行的任务被标记为 `failed`
3. 任务自动重新推入队列
4. 其他 Worker 会继续处理

### Q: 如何实现任务优先级？

**A**: 设置 `Priority` 字段，数值越大优先级越高：

```go
// 高优先级任务
urgentTask := &task.TaskEntity{
    TaskType: "urgent_notification",
    Priority: 100,
    // ...
}

// 普通任务
normalTask := &task.TaskEntity{
    TaskType: "send_email",
    Priority: 0,
    // ...
}
```

### Q: 能否在不停止 Worker 的情况下动态注册执行器？

**A**: 不建议。执行器应该在 Worker 启动前注册。如果需要动态添加任务类型，建议：
1. 部署新的 Worker 实例并注册新执行器
2. 或者重启现有 Worker（在低峰期进行）

## 示例代码

更多示例代码请查看 [`examples/`](examples/) 目录：

- **basic**: 基本任务创建和执行
- **retry**: 任务重试机制
- **timeout**: 任务超时处理
- **concurrent**: 并发任务处理
- **steps**: 步骤化任务流程

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT License
