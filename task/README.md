# Task - 分布式任务队列

`task` 是一个基于 Redis Stream 的分布式任务队列系统，提供可靠的任务调度和执行能力。

## 特性

- **分布式架构**: 基于 Redis Stream 实现分布式任务队列，支持多 Worker 并发处理
- **任务状态管理**: 支持 pending、running、success、failed、canceled、timeout 等状态
- **自动重试**: 任务失败后自动重试，支持配置最大重试次数
- **超时控制**: 任务执行超时自动标记并支持重试
- **步骤化执行**: 支持多步骤任务流程，前置步骤未完成时后续步骤不执行
- **优先级调度**: 支持任务优先级，高优先级任务优先执行
- **心跳检查**: Worker 定期上报心跳，自动检测故障并恢复任务
- **并发控制**: 可配置每个 Worker 的最大并发数，支持不同任务类型设置不同并发数
- **多租户支持**: 支持 CompanyID 和 Uin 字段，适用于多租户场景
- **模块化设计**: 清晰的职责分离，Worker、Manager、HealthChecker 独立运行

## 安装

```bash
go get github.com/ygpkg/yg-go/task
```

## 架构设计

### 包结构

```
task/
├── worker/              # Worker 子包 - 任务执行核心
│   ├── worker.go        # Worker 实现
│   ├── executor.go      # 执行器接口和注册表
│   ├── config.go        # Worker 配置
│   ├── errors.go        # Worker 错误定义
│   └── executor_test.go # 执行器测试
├── manager/             # Manager 子包 - 任务管理门面
│   ├── manager.go       # 任务管理器（CRUD + 队列）
│   ├── queue.go         # Redis Stream 队列
│   ├── repository.go    # 数据访问层
│   ├── config.go        # Manager 配置
│   └── init.go          # 数据库初始化
├── health/              # Health 子包 - 健康检查
│   ├── checker.go       # 健康检查器实现
│   └── config.go        # 健康检查配置
├── model/               # Model 子包 - 数据模型
│   ├── entity.go        # 任务实体
│   ├── dao.go           # 数据访问对象
│   └── errors.go        # 错误定义
├── README.md
└── example/             # 示例代码
```

### 依赖关系

```
业务层             →  manager.Manager（任务管理）
业务层             →  worker.Worker（任务执行）
worker.Worker      →  manager.Manager（内部使用，拉取和保存任务）
health.Checker     →  manager.Manager（内部使用）
manager.Manager    →  使用 Queue 和 Repository
manager, health    →  model（数据模型）
worker             →  不依赖 model（与业务解耦）
```

### 核心组件

#### 1. Manager（任务管理门面）

任务管理的统一入口，提供完整的任务管理 API：

- **任务 CRUD**: 创建、查询、更新、取消任务
- **队列操作**: 推送和消费队列消息
- **任务查询**: 获取待处理任务、统计任务数量等
- **状态管理**: 初始化任务状态、检查超时任务

#### 2. Worker（任务执行器）

纯粹的任务执行引擎，**不暴露任务管理接口**：

- 从队列拉取任务
- 调用执行器工厂创建执行器并执行任务
- 处理任务结果（成功/失败/超时）
- 触发下一个任务
- **不直接操作** DB 和 Redis，通过 Manager 调用

#### 3. HealthChecker（健康检查器）

独立的健康检查服务：

- 监控 Worker 心跳
- 检测故障 Worker 并恢复任务
- 同步队列状态
- **独立启动/停止**，不与 Worker 生命周期绑定

#### 4. Model（数据模型）

统一的数据模型层：

- **TaskEntity**: 任务实体定义
- **TaskDao**: 数据访问对象
- **TaskStatus**: 任务状态枚举
- **错误定义**: 统一的错误类型

## 核心概念

### TaskEntity 结构

任务实体包含以下主要字段（位于 `model` 包）：

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
    AppGroup    string        // 应用分组（用于步骤化任务）
}
```

### 任务执行器

任务执行器需要实现 `TaskExecutor` 接口（位于 `worker` 包）：

```go
type TaskExecutor interface {
    Execute(ctx context.Context) error
    OnSuccess(ctx context.Context, tx *gorm.DB) error
    OnFailure(ctx context.Context, tx *gorm.DB) error
}
```

执行器通过工厂函数创建，工厂函数接收 payload 参数：

```go
type ExecutorFactory func(payload string) (TaskExecutor, error)
```

## 快速开始

### 1. 初始化数据库表

```go
import (
    "github.com/ygpkg/yg-go/task/manager"
    "gorm.io/gorm"
)

func InitDB(db *gorm.DB) error {
    return manager.Init(db)
}
```

### 2. 定义任务执行器

```go
import (
    "context"
    "encoding/json"
    "fmt"

    "github.com/ygpkg/yg-go/task/worker"
    "gorm.io/gorm"
)

// DemoPayload 任务参数
type DemoPayload struct {
    Message string `json:"message"`
    UserID  int    `json:"user_id"`
}

// DemoTaskExecutor 示例任务执行器
type DemoTaskExecutor struct {
    payload DemoPayload
}

// NewDemoTaskExecutor 创建执行器（在工厂函数中解析参数）
func NewDemoTaskExecutor(payloadJSON string) (*DemoTaskExecutor, error) {
    var payload DemoPayload
    if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
        return nil, fmt.Errorf("failed to parse payload: %w", err)
    }
    return &DemoTaskExecutor{payload: payload}, nil
}

func (e *DemoTaskExecutor) Execute(ctx context.Context) error {
    // 执行任务逻辑
    fmt.Printf("Processing task: %s for user %d\n", e.payload.Message, e.payload.UserID)
    return nil
}

func (e *DemoTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
    return nil
}

func (e *DemoTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
    return nil
}
```

### 3. 创建并启动服务

```go
package main

import (
    "context"
    "time"

    "github.com/ygpkg/yg-go/task/model"
    "github.com/ygpkg/yg-go/task/worker"
    "github.com/ygpkg/yg-go/task/manager"
    "github.com/ygpkg/yg-go/task/health"
)

func main() {
    // 假设已经初始化好 db 和 redisClient

    ctx := context.Background()

    // 1. 创建任务管理器（任务管理的统一入口）
    managerConfig := &manager.ManagerConfig{
        KeyPrefix:      "task:",
        QueueBlockTime: 5 * time.Second,
    }
    taskMgr, err := manager.NewManager(managerConfig, db, redisClient)
    if err != nil {
        panic(err)
    }

    // 2. 创建 Worker（只负责任务执行）
    workerConfig := &worker.WorkerConfig{
        WorkerID:       "worker-001",
        Timeout:        10 * time.Minute,
        MaxRedo:        3,
        MaxConcurrency: 5,
    }
    w, err := worker.NewWorker(workerConfig, taskMgr, db)
    if err != nil {
        panic(err)
    }

    // 3. 注册执行器（通过工厂函数，在创建时解析参数）
    w.RegisterExecutor("demo_task", func(payload string) (worker.TaskExecutor, error) {
        return NewDemoTaskExecutor(payload)
    })

    // 4. 创建健康检查器（独立运行）
    healthConfig := &health.CheckerConfig{
        KeyPrefix:   "task:",
        RedisClient: redisClient,
        Manager:     taskMgr,
        CheckPeriod: 30 * time.Second,
    }
    healthChecker, err := health.NewChecker(healthConfig)
    if err != nil {
        panic(err)
    }

    // 5. 启动服务
    if err := healthChecker.Start(ctx); err != nil {
        panic(err)
    }

    if err := w.Start(ctx); err != nil {
        panic(err)
    }

    // 6. 通过 Manager 创建任务（不通过 Worker）
    payload, _ := json.Marshal(DemoPayload{
        Message: "Hello Task!",
        UserID:  12345,
    })

    taskEntity := &model.TaskEntity{
        TaskType:    "demo_task",
        SubjectType: "demo",
        SubjectID:   1,
        Payload:     string(payload),
        Timeout:     5 * time.Minute,
        MaxRedo:     3,
        Priority:    0,
        CompanyID:   1,
        Uin:         1001,
    }

    if err := taskMgr.CreateTask(ctx, taskEntity); err != nil {
        panic(err)
    }

    // 等待任务完成...

    // 7. 停止服务
    w.Stop(ctx)
    healthChecker.Stop(ctx)
}
```

## 高级特性

### 1. 混合并发控制

不同任务类型可以设置不同的并发数：

```go
// 高并发任务
w.RegisterExecutor("fast_task", func(payload string) (worker.TaskExecutor, error) {
    return NewFastTaskExecutor(payload)
}, worker.WithConcurrency(10))

// 低并发任务（资源密集型）
w.RegisterExecutor("heavy_task", func(payload string) (worker.TaskExecutor, error) {
    return NewHeavyTaskExecutor(payload)
}, worker.WithConcurrency(2))
```

### 2. 步骤化任务

使用 `AppGroup` 和 `Step` 字段实现多步骤任务流程：

```go
tasks := []*model.TaskEntity{
    {
        TaskType:  "step_1",
        SubjectID: 1,
        AppGroup:  "order_process",
        Step:      1, // 第一步
        // ...
    },
    {
        TaskType:  "step_2",
        SubjectID: 1,
        AppGroup:  "order_process",
        Step:      2, // 第二步（需等待第一步完成）
        // ...
    },
}

taskMgr.CreateTasks(ctx, tasks)
```

### 3. 任务重试

任务失败后会自动重试：

```go
taskEntity := &model.TaskEntity{
    TaskType: "retry_task",
    MaxRedo:  3, // 最多重试 3 次
    // ...
}
```

### 4. 超时控制

任务执行时间超过 Timeout 会自动标记为超时：

```go
taskEntity := &model.TaskEntity{
    TaskType: "timeout_task",
    Timeout:  30 * time.Second, // 30 秒超时
    // ...
}
```

## 示例代码

完整的示例代码请参考 [example](./example) 目录，包含：

1. **基本任务创建和执行**
2. **任务重试机制**
3. **任务超时处理**
4. **并发任务处理**
5. **步骤化任务流程**
6. **混合并发配置**

运行示例：

```bash
cd example
go run .
```

## 最佳实践

### 1. 职责分离

- **Manager**: 任务管理的统一入口，负责任务 CRUD 和队列管理
- **Worker**: 纯粹的任务执行器，不暴露任务管理接口
- **HealthChecker**: 独立运行健康检查

### 2. 任务管理通过 Manager

所有任务的创建、查询、取消等操作都应通过 Manager，而不是 Worker：

```go
// 正确：通过 Manager 创建任务
taskMgr.CreateTask(ctx, taskEntity)

// 正确：通过 Manager 查询任务
task, err := taskMgr.GetTask(ctx, taskID)

// Worker 只负责注册执行器和启停
w.RegisterExecutor("task_type", factory)
w.Start(ctx)
```

### 3. 独立部署

健康检查器可以独立部署，不依赖 Worker：

```go
// 只部署健康检查器
healthChecker.Start(ctx)
// 不启动 Worker
```

### 4. 错误处理

执行器中应该妥善处理错误：

```go
func (e *MyExecutor) Execute(ctx context.Context) error {
    // 检查上下文取消
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }

    // 执行业务逻辑
    if err := doSomething(); err != nil {
        return fmt.Errorf("failed to do something: %w", err)
    }

    return nil
}
```

### 5. 事务处理

在 OnSuccess/OnFailure 中使用事务：

```go
func (e *MyExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
    // 在同一事务中更新相关数据
    return tx.Model(&MyModel{}).Where("id = ?", e.subjectID).
        Update("status", "completed").Error
}
```

## 注意事项

1. **数据库表名**: 任务表名固定为 `core_task`
2. **Redis 键前缀**: 建议为不同环境设置不同的键前缀，避免冲突
3. **WorkerID**: 每个 Worker 实例应该有唯一的 WorkerID
4. **并发数**: 根据服务器资源合理配置并发数
5. **心跳超时**: 默认 30 秒，Worker 崩溃后任务会在 30 秒后被恢复

## License

MIT
