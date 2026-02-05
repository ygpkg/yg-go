# TaskQueue - 统一的分布式任务队列

`task` 是一个通用的任务队列包，提供两种模式的任务调度和执行：

- **分布式任务模式 (disttask)**: 基于 Redis Stream 的分布式任务队列，支持多 Worker 并发处理
- **本地任务模式 (localtask)**: 本地协程轮询数据库，适用于中小规模任务处理

## 特性

### 通用特性
- 统一的任务执行器接口
- 任务状态管理（pending, running, success, failed, canceled, timeout）
- 任务重试机制
- 任务超时控制
- 任务优先级
- 步骤化任务执行
- 父子任务关系

### 分布式模式特性
- Redis Stream 队列
- Worker 心跳检查
- 健康状态监控
- 分布式任务分发
- 自动故障恢复

### 本地模式特性
- Redis 分布式锁
- 协程池管理
- 轮询调度
- 简单易用

## 安装

```bash
# 安装基础包
go get github.com/ygpkg/yg-go/task

# 按需导入具体实现
# 分布式任务模式
go get github.com/ygpkg/yg-go/task/disttask

# 本地任务模式
go get github.com/ygpkg/yg-go/task/localtask
```

## 包结构说明

为了避免循环依赖，本包采用按需导入的设计：

- **`task`**: 核心包，定义了任务模型、接口和类型
- **`task/disttask`**: 分布式任务实现，基于 Redis Stream
- **`task/localtask`**: 本地任务实现，基于协程轮询

**使用时直接导入需要的实现包**，例如：
```go
import "github.com/ygpkg/yg-go/task/disttask"  // 使用分布式模式
import "github.com/ygpkg/yg-go/task/localtask" // 使用本地模式
```

## 快速开始

### 1. 定义任务执行器

```go
package main

import (
	"context"
	"fmt"
	
	"github.com/ygpkg/yg-go/task"
	"gorm.io/gorm"
)

// MyTaskExecutor 自定义任务执行器
type MyTaskExecutor struct {
	task.BaseExecutor
	data string
}

// Prepare 初始化执行器
func (e *MyTaskExecutor) Prepare(ctx context.Context, task *task.Task) error {
	// 调用基类 Prepare
	if err := e.BaseExecutor.Prepare(ctx, task); err != nil {
		return err
	}
	
	// 解析任务数据
	e.data = task.Payload
	return nil
}

// Execute 执行任务
func (e *MyTaskExecutor) Execute(ctx context.Context) error {
	// 执行具体的业务逻辑
	fmt.Printf("Processing task: %s\n", e.data)
	// ... 你的业务逻辑 ...
	return nil
}

// OnSuccess 成功回调（可选）
func (e *MyTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	// 任务成功后的操作，如更新数据库
	return nil
}

// OnFailure 失败回调（可选）
func (e *MyTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	// 任务失败后的操作，如记录日志
	return nil
}
```

### 2. 本地模式使用示例

```go
package main

import (
	"context"
	"time"
	
	"github.com/ygpkg/yg-go/task"
	"github.com/ygpkg/yg-go/task/localtask"
	"gorm.io/gorm"
)

func main() {
	// 配置
	config := &task.TaskConfig{
		Mode:           task.ModeLocal,
		DBInstance:     "default",  // 数据库实例名
		MaxConcurrency: 5,           // 并发数
		PollInterval:   5 * time.Second,  // 轮询间隔
		Timeout:        10 * time.Minute, // 默认超时
		MaxRedo:        3,                // 最大重试次数
		RedisKeyPrefix: "task:",     // Redis 键前缀
	}
	
	// 创建本地调度器（按需导入）
	scheduler, err := localtask.NewSchedulerWithDBInstance(config)
	if err != nil {
		panic(err)
	}
	
	// 注册任务执行器
	scheduler.RegisterExecutor("my_task_type", func() task.TaskExecutor {
		return &MyTaskExecutor{}
	})
	
	// 启动调度器
	ctx := context.Background()
	if err := scheduler.Start(ctx); err != nil {
		panic(err)
	}
	defer scheduler.Stop(ctx)
	
	// 创建任务
	task := &task.Task{
		TaskType:  "my_task_type",
		SubjectID: 123,
		Payload:   `{"key": "value"}`,
		Timeout:   5 * time.Minute,
		MaxRedo:   3,
	}
	
	if err := scheduler.CreateTask(ctx, task); err != nil {
		panic(err)
	}
	
	// 等待任务完成...
}
```

### 3. 分布式模式使用示例

```go
package main

import (
	"context"
	"time"
	
	"github.com/ygpkg/yg-go/task"
	"github.com/ygpkg/yg-go/task/disttask"
)

func main() {
	// 配置
	config := &task.TaskConfig{
		Mode:              task.ModeDistributed,
		DBInstance:        "default",
		WorkerID:          "worker-001",  // Worker 标识
		MaxConcurrency:    10,
		HealthCheckPeriod: 30 * time.Second,
		Timeout:           10 * time.Minute,
		MaxRedo:           3,
		RedisKeyPrefix:    "task:",
		EnableHealthCheck: true,
	}
	
	// 创建分布式 Worker（按需导入）
	worker, err := disttask.NewWorkerWithDBInstance(config)
	if err != nil {
		panic(err)
	}
	
	// 注册任务执行器
	worker.RegisterExecutor("my_task_type", func() task.TaskExecutor {
		return &MyTaskExecutor{}
	})
	
	// 启动 Worker
	ctx := context.Background()
	if err := worker.Start(ctx); err != nil {
		panic(err)
	}
	defer worker.Stop(ctx)
	
	// 创建任务（通常在其他服务中创建）
	task := &task.Task{
		TaskType:  "my_task_type",
		SubjectID: 123,
		Payload:   `{"key": "value"}`,
		Timeout:   5 * time.Minute,
		MaxRedo:   3,
	}
	
	if err := worker.CreateTask(ctx, task); err != nil {
		panic(err)
	}
	
	// Worker 会自动拉取并执行任务...
}
```

## 高级用法

### 步骤化任务

```go
// 创建多个步骤的任务
tasks := []*task.Task{
	{
		TaskType:  "step1_task",
		SubjectID: 123,
		AppGroup:  "process_flow",
		Step:      1,  // 步骤 1
		Payload:   `{"data": "step1"}`,
		Timeout:   5 * time.Minute,
		MaxRedo:   3,
	},
	{
		TaskType:  "step2_task",
		SubjectID: 123,
		AppGroup:  "process_flow",
		Step:      2,  // 步骤 2，会等待步骤 1 完成
		Payload:   `{"data": "step2"}`,
		Timeout:   5 * time.Minute,
		MaxRedo:   3,
	},
}

for _, task := range tasks {
	if err := manager.CreateTask(ctx, task); err != nil {
		panic(err)
	}
}
```

### 父子任务

```go
// 创建父任务
parentTask := &task.Task{
	TaskType:  "parent_task",
	SubjectID: 123,
	Payload:   `{"type": "parent"}`,
	Timeout:   10 * time.Minute,
	MaxRedo:   3,
}

if err := manager.CreateTask(ctx, parentTask); err != nil {
	panic(err)
}

// 创建子任务
childTask := &task.Task{
	TaskType:  "child_task",
	SubjectID: 123,
	ParentID:  parentTask.ID,  // 关联父任务
	Payload:   `{"type": "child"}`,
	Timeout:   5 * time.Minute,
	MaxRedo:   3,
}

if err := manager.CreateTask(ctx, childTask); err != nil {
	panic(err)
}
```

### 查询任务状态

```go
// 获取任务信息
task, err := manager.GetTask(ctx, taskID)
if err != nil {
	panic(err)
}

fmt.Printf("Task Status: %s\n", task.TaskStatus)
fmt.Printf("Task Result: %s\n", task.Result)
fmt.Printf("Task Error: %s\n", task.ErrMsg)
```

### 取消任务

```go
// 取消任务
err := manager.CancelTask(ctx, taskID, "user requested")
if err != nil {
	panic(err)
}
```

## 配置说明

### TaskConfig 字段

| 字段 | 类型 | 说明 | 默认值 |
|------|------|------|--------|
| Mode | TaskMode | 任务模式：distributed 或 local | local |
| DBInstance | string | 数据库实例名称 | "" |
| Timeout | time.Duration | 默认超时时间 | 10分钟 |
| MaxRedo | int | 默认重试次数 | 3 |
| MaxConcurrency | int | 最大并发数 | 5 |
| PollInterval | time.Duration | 轮询间隔（本地模式） | 5秒 |
| HealthCheckPeriod | time.Duration | 健康检查周期（分布式模式） | 30秒 |
| RedisKeyPrefix | string | Redis 键前缀 | "task:" |
| WorkerID | string | Worker 标识（分布式模式必填） | "" |
| EnableHealthCheck | bool | 是否启用健康检查（分布式模式） | true |

## 数据库表结构

任务数据存储在 `core_tasks` 表中：

```sql
CREATE TABLE `core_tasks` (
  `id` bigint unsigned NOT NULL AUTO_INCREMENT,
  `created_at` datetime(3) DEFAULT NULL,
  `updated_at` datetime(3) DEFAULT NULL,
  `deleted_at` datetime(3) DEFAULT NULL,
  `task_type` varchar(64) NOT NULL,
  `task_status` varchar(20) NOT NULL,
  `subject_id` bigint NOT NULL,
  `priority` int NOT NULL DEFAULT '0',
  `step` int NOT NULL DEFAULT '0',
  `redo` int NOT NULL DEFAULT '0',
  `max_redo` int NOT NULL DEFAULT '3',
  `timeout` bigint NOT NULL,
  `payload` text,
  `result` text,
  `err_msg` text,
  `worker_id` varchar(64) DEFAULT NULL,
  `start_at` datetime DEFAULT NULL,
  `end_at` datetime DEFAULT NULL,
  `cost` bigint DEFAULT '0',
  `parent_id` bigint DEFAULT '0',
  `company_id` bigint NOT NULL DEFAULT '0',
  `uin` bigint NOT NULL DEFAULT '0',
  `app_group` varchar(32) DEFAULT NULL,
  PRIMARY KEY (`id`),
  KEY `idx_task_type` (`task_type`),
  KEY `idx_task_status` (`task_status`),
  KEY `idx_subject_id` (`subject_id`),
  KEY `idx_priority` (`priority`),
  KEY `idx_parent_id` (`parent_id`),
  KEY `idx_company_id` (`company_id`),
  KEY `idx_uin` (`uin`),
  KEY `idx_app_group` (`app_group`),
  KEY `idx_deleted_at` (`deleted_at`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

初始化数据库表：

```go
import (
	"github.com/ygpkg/yg-go/task"
	"gorm.io/gorm"
)

func InitDB(db *gorm.DB) error {
	return task.InitDB(db)
}
```

## 最佳实践

### 1. 任务执行器设计

- 保持任务执行器无状态，避免在执行器中存储全局状态
- 将任务数据序列化为 JSON 存储在 `Payload` 字段
- 在 `Prepare` 方法中进行初始化和参数验证
- 在 `Execute` 方法中实现核心业务逻辑
- 使用 `OnSuccess` 和 `OnFailure` 回调处理事务性操作

### 2. 错误处理

- 返回明确的错误信息，便于调试
- 区分可重试和不可重试的错误
- 使用适当的重试次数，避免无限重试

### 3. 超时设置

- 根据任务的实际执行时间设置合理的超时时间
- 对于长时间运行的任务，适当增加超时时间
- 在任务执行过程中定期检查 context 是否取消

### 4. 并发控制

- 根据系统资源合理设置并发数
- 对于 CPU 密集型任务，并发数不宜过高
- 对于 I/O 密集型任务，可以适当增加并发数

### 5. 模式选择

**选择分布式模式的场景：**
- 任务量大，需要多机器并行处理
- 需要横向扩展能力
- 需要高可用性和故障恢复
- 任务执行时间较长

**选择本地模式的场景：**
- 任务量不大，单机可以处理
- 对延迟要求不高
- 系统架构简单，不需要分布式部署
- 开发和测试环境

## 监控和运维

### 查询任务统计

```go
// 获取待处理任务数量（本地模式）
import "github.com/ygpkg/yg-go/task/localtask"

scheduler, _ := localtask.NewSchedulerWithDBInstance(config)
count, err := scheduler.GetPendingCount(ctx, "my_task_type")
if err != nil {
	panic(err)
}
fmt.Printf("Pending tasks: %d\n", count)
```

### 健康检查（分布式模式）

分布式模式会自动进行健康检查，检测 Worker 心跳超时并自动恢复任务。

### 日志

任务队列使用 `yg-go/logs` 包记录日志，包括：
- 任务创建日志
- 任务执行日志
- 错误和警告日志
- 健康检查日志

## 常见问题

### Q: 如何选择合适的模式？

A: 如果你的应用需要横向扩展，处理大量任务，选择分布式模式。如果是中小规模应用，本地模式更简单易用。

### Q: 任务重试机制如何工作？

A: 当任务执行失败或超时时，系统会自动增加 `redo` 计数。只要 `redo < max_redo`，任务就会被重新调度执行。

### Q: 如何确保任务不会被重复执行？

A: 分布式模式使用 Redis Stream Consumer Group 确保消息只被消费一次。本地模式使用 Redis 分布式锁和数据库行锁防止并发。

### Q: 任务超时后会发生什么？

A: 超时的任务会被标记为 `timeout` 状态，`redo` 计数加 1。如果还可以重试，会被重新调度。

### Q: 如何处理长时间运行的任务？

A: 为任务设置足够长的 `Timeout`，并在执行过程中定期检查 `context.Done()`，以支持优雅退出。

## 贡献

欢迎提交 Issue 和 Pull Request！

## 许可证

MIT License
