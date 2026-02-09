# Task 包使用示例

本目录提供 Task 包的交互式使用示例，帮助你快速上手并理解核心功能。

## 快速开始

### 前置条件

运行示例前需要确保以下服务已启动：

#### 1. MySQL

```bash
# 使用 Docker 启动 MySQL
docker run -d \
  --name mysql-task-example \
  -e MYSQL_ROOT_PASSWORD=123456 \
  -e MYSQL_DATABASE=demo \
  -p 3306:3306 \
  mysql:8.0
```

#### 2. Redis

```bash
# 使用 Docker 启动 Redis
docker run -d \
  --name redis-task-example \
  -p 6379:6379 \
  redis:7-alpine
```

#### 3. 配置数据库连接

示例代码使用的默认配置：

- **Host**: localhost
- **Port**: 3306
- **User**: root
- **Password**: 123456
- **Database**: demo

如需修改配置，请编辑 `util.go` 文件中的 `setupDB()` 函数。

### 运行示例

```bash
cd example
go run .
```

## 示例列表

运行后会显示交互式菜单，可以选择以下示例：

### 1. 基本任务创建和执行

演示最基本的任务队列使用方法：
- 定义简单的任务执行器
- 创建和启动 Worker
- 创建任务并执行
- 查询任务状态

**学习要点**：
- `TaskExecutor` 接口的实现
- Worker 的创建和配置
- 任务的创建和生命周期

### 2. 任务重试机制

演示任务失败后的自动重试机制：
- 创建会失败的任务
- 配置重试次数
- 观察自动重试过程
- 最终成功的处理

**学习要点**：
- 任务重试机制的工作原理
- `MaxRedo` 参数的使用
- 失败任务的状态变化

### 3. 任务超时处理

演示任务超时检测和处理：
- 设置任务超时时间
- 模拟长时间运行的任务
- 超时检测和任务取消
- Context 取消的正确处理

**学习要点**：
- 如何设置任务超时时间
- Context 取消信号的处理
- 超时任务的重试机制

### 4. 并发任务处理

演示批量任务的并发处理：
- 批量创建任务
- 配置并发数
- 监控并发执行情况
- 性能统计

**学习要点**：
- 批量创建任务的方法
- `MaxConcurrency` 参数的影响
- 并发任务的性能优化

### 5. 步骤化任务流程

演示多步骤任务的顺序执行：
- 创建多步骤任务
- 步骤依赖关系
- 顺序执行验证
- 步骤失败处理

**学习要点**：
- `Step` 和 `AppGroup` 的使用
- 步骤化任务的执行顺序
- 前置步骤失败的处理

### 6. 混合并发（不同任务类型不同并发数）

演示不同任务类型使用不同并发数的场景：
- 为不同任务类型配置独立的并发数
- 快速任务使用高并发（10）提升吞吐量
- 慢速任务使用低并发（2）避免资源耗尽
- API 调用任务使用中等并发（5）
- 向后兼容的默认并发配置

**学习要点**：
- 使用 `task.WithConcurrency()` 选项指定并发数
- 不传选项时使用全局默认值（向后兼容）
- 根据任务特性（CPU密集型 vs IO密集型）合理配置并发数
- 不同任务类型独立并发，互不影响

**适用场景**：
- CPU密集型任务：使用低并发避免CPU过载
- IO密集型任务：使用高并发提升资源利用率
- 限流需求任务：精确控制对下游服务的并发请求
- 不同优先级任务：核心业务高并发，辅助业务低并发

**示例代码**：
```go
// 为不同任务类型配置独立并发数（使用 WithConcurrency 选项）
worker.RegisterExecutor("fast_task", func() task.TaskExecutor {
    return &FastTaskExecutor{}
}, task.WithConcurrency(10)) // 快速任务高并发

worker.RegisterExecutor("slow_task", func() task.TaskExecutor {
    return &SlowTaskExecutor{}
}, task.WithConcurrency(2)) // 慢速任务低并发

// 使用全局默认并发数（不传选项）
worker.RegisterExecutor("default_task", func() task.TaskExecutor {
    return &DefaultTaskExecutor{}
})
```

## 代码结构

```
example/
├── main.go         # 统一入口和交互式菜单
├── executors.go    # 所有任务执行器实现
├── scenarios.go    # 各个使用场景的函数实现
├── util.go         # 工具函数（数据库、Redis 连接等）
└── README.md       # 本文档
```

### 文件说明

- **main.go**: 程序入口，实现交互式菜单，根据用户选择运行不同场景
- **executors.go**: 包含所有任务执行器的实现，每个场景对应一个或多个执行器
- **scenarios.go**: 包含各个使用场景的函数实现，每个函数演示一个特定功能
- **util.go**: 提供通用工具函数，如数据库连接、Redis 连接等

## 通用配置

所有示例使用相同的配置结构，不同场景会根据需要调整部分参数：

```go
config := &task.TaskConfig{
    WorkerID:          "example-worker-001", // Worker 唯一标识
    MaxConcurrency:    5,                    // 最大并发数
    Timeout:           10 * time.Minute,
    MaxRedo:           3,
    RedisKeyPrefix:    "task:example:",      // Redis 键前缀
    EnableHealthCheck: true,
    HealthCheckPeriod: 30 * time.Second,
}
```

## 故障排查

### 无法连接数据库

```
Error: failed to connect database
```

**解决方法**：
1. 确保 MySQL 服务正在运行
2. 检查数据库连接配置（用户名、密码、数据库名）
3. 确保数据库已创建（demo）

### 无法连接 Redis

```
Error: failed to connect redis
```

**解决方法**：
1. 确保 Redis 服务正在运行
2. 检查 Redis 地址配置（默认 localhost:6379）
3. 检查 Redis 是否需要密码

### 任务不执行

1. 检查 Worker 是否成功启动
2. 检查是否注册了对应的任务执行器
3. 检查 Redis Stream 队列是否有消息

```bash
# 查看 Redis Stream 消息
redis-cli
> XINFO STREAM task:example:task_queue:demo_task
```

## 清理数据

测试完成后，可以清理测试数据：

```sql
-- 清理任务表
TRUNCATE TABLE core_task;
```

```bash
# 清理 Redis 数据
redis-cli FLUSHDB
```

## 停止服务

```bash
# 停止 MySQL
docker stop mysql-task-example
docker rm mysql-task-example

# 停止 Redis
docker stop redis-task-example
docker rm redis-task-example
```

## 更多资源

- [Task 包文档](../README.md) - 完整的 API 文档和设计说明
- [最佳实践](../README.md#最佳实践) - 生产环境使用建议

## 反馈

如果你在运行示例时遇到问题，或者有改进建议，欢迎提交 Issue！
