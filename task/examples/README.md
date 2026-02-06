# Task 包使用示例

本目录包含 Task 包的各种使用示例，帮助你快速上手并理解核心功能。

## 前置条件

运行示例前需要确保以下服务已启动：

### 1. MySQL

```bash
# 使用 Docker 启动 MySQL
docker run -d \
  --name mysql-task-examples \
  -e MYSQL_ROOT_PASSWORD=root \
  -e MYSQL_DATABASE=demo \
  -p 3306:3306 \
  mysql:8.0
```

### 2. Redis

```bash
# 使用 Docker 启动 Redis
docker run -d \
  --name redis-task-examples \
  -p 6379:6379 \
  redis:7-alpine
```

### 3. 配置数据库连接

示例代码使用硬编码的数据库连接配置：

- **Host**: localhost
- **Port**: 3306
- **User**: root
- **Password**: root
- **Database**: demo

如需修改配置，请编辑各示例文件中的 `setupDB()` 函数。

```go
// setupDB 函数在每个示例文件中都有定义
func setupDB() (*gorm.DB, error) {
    dsn := "root:root@tcp(localhost:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local"
    
    db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
        CreateBatchSize: 200,
    })
    if err != nil {
        return nil, fmt.Errorf("failed to connect database: %w", err)
    }
    
    return db, nil
}
```

配置 Redis（示例代码仍需要 redispool）：

```go
import "github.com/ygpkg/yg-go/dbtools/redispool"

// 初始化 Redis
func init() {
    redispool.InitRedis(&redispool.RedisConfig{
        Addr: "localhost:6379",
    })
}
```

## 示例列表

### [basic](basic/) - 基本任务创建和执行

演示最基本的任务队列使用方法：
- 定义简单的任务执行器
- 创建和启动 Worker
- 创建任务并执行
- 查询任务状态

**运行方法**：
```bash
cd basic
go run main.go
```

**学习要点**：
- TaskExecutor 接口的实现
- Worker 的创建和配置
- 任务的创建和生命周期

---

### [retry](retry/) - 任务重试机制

演示任务失败后的自动重试机制：
- 创建会失败的任务
- 配置重试次数
- 观察自动重试过程
- 最终成功的处理

**运行方法**：
```bash
cd retry
go run main.go
```

**学习要点**：
- 任务重试机制的工作原理
- MaxRedo 参数的使用
- 失败任务的状态变化

---

### [timeout](timeout/) - 任务超时处理

演示任务超时检测和处理：
- 设置任务超时时间
- 模拟长时间运行的任务
- 超时检测和任务取消
- Context 取消的正确处理

**运行方法**：
```bash
cd timeout
go run main.go
```

**学习要点**：
- 如何设置任务超时时间
- Context 取消信号的处理
- 超时任务的重试机制

---

### [concurrent](concurrent/) - 并发任务处理

演示批量任务的并发处理：
- 批量创建任务
- 配置并发数
- 监控并发执行情况
- 性能统计

**运行方法**：
```bash
cd concurrent
go run main.go
```

**学习要点**：
- 批量创建任务的方法
- MaxConcurrency 参数的影响
- 并发任务的性能优化

---

### [steps](steps/) - 步骤化任务流程

演示多步骤任务的顺序执行：
- 创建多步骤任务
- 步骤依赖关系
- 顺序执行验证
- 步骤失败处理

**运行方法**：
```bash
cd steps
go run main.go
```

**学习要点**：
- Step 和 AppGroup 的使用
- 步骤化任务的执行顺序
- 前置步骤失败的处理

---

## 通用配置

所有示例使用相同的配置结构：

```go
config := &task.TaskConfig{
    WorkerID:          "worker-001", // Worker 唯一标识
    MaxConcurrency:    5,            // 最大并发数
    Timeout:           10 * time.Minute,
    MaxRedo:           3,
    RedisKeyPrefix:    "task:demo:", // Redis 键前缀
    EnableHealthCheck: true,
    HealthCheckPeriod: 30 * time.Second,
}
```

## 日志输出

所有示例都会输出详细的日志，帮助理解任务的执行过程：

```
[task] created task, id: 1, type: demo_task
[task] worker started, workerID: worker-001
[task] push task to queue, taskType: demo_task
[task] pop task from queue, taskType: demo_task, workerID: worker-001
[task] task success
```

## 故障排查

### 无法连接数据库

```
Error: failed to create worker: database instance not found: default
```

**解决方法**：确保已初始化 dbtools，参考前置条件部分。

### 无法连接 Redis

```
Error: failed to push task to queue: redis connection refused
```

**解决方法**：确保 Redis 服务正在运行，并且地址配置正确。

### 任务不执行

1. 检查 Worker 是否成功启动
2. 检查是否注册了对应的任务执行器
3. 检查 Redis Stream 队列是否有消息

```bash
# 查看 Redis Stream 消息
redis-cli
> XINFO STREAM task:demo:task_queue:demo_task
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

## 更多资源

- [主文档](../README.md) - Task 包完整文档
- [API 参考](../README.md#api-参考) - 接口和方法说明
- [最佳实践](../README.md#最佳实践) - 生产环境使用建议

## 反馈

如果你在运行示例时遇到问题，或者有改进建议，欢迎提交 Issue！
