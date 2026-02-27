# Task 包使用示例

本目录提供 Task 包的交互式使用示例，帮助你快速上手并理解核心功能。

## 快速开始

### 前置条件

运行示例前需要确保以下服务已启动：

#### MySQL

确保 MySQL 服务正在运行，并创建数据库：

```sql
CREATE DATABASE demo;
```

#### Redis

确保 Redis 服务正在运行。

#### 配置数据库连接

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

## 示例说明

本示例包含 4 个测试场景，全面演示 Task 包的核心功能：

### 测试 1: 正常任务执行

**目的**: 演示最基本的任务创建和执行流程。

**测试内容**:
- 定义简单的任务执行器
- 创建和启动 Worker
- 创建任务并执行
- 查询任务状态

**学习要点**:
- `TaskExecutor` 接口的实现
- Worker 的创建和配置
- 任务的创建和生命周期

**等待时间**: 约 5 秒

---

### 测试 2: 超时检测和重试

**目的**: 演示任务超时检测机制和自动重试。

**测试内容**:
- 创建一个超时配置的任务（Timeout=3秒，实际执行=5秒）
- 演示 `timeoutCheckRoutine` 检测超时
- 演示超时任务根据 `MaxRedo` 自动重试

**学习要点**:
- 超时任务的执行过程
- `timeoutCheckRoutine` 的工作原理（每分钟检查一次）
- 超时任务的重试机制
- `CanRetry()` 方法的作用

**预期过程**:
1. 任务开始执行（running 状态）
2. 等待 `timeoutCheckRoutine` 检测（最多 60 秒）
3. 检测到超时后标记为 timeout，重试次数 +1
4. 根据 `CanRetry()` 自动重新入队
5. Worker 重新获取并执行
6. 重试后仍然超时，最终状态为 timeout（Redo=1/1）

**等待时间**: 约 60-90 秒

---

### 测试 3: 失败任务重试

**目的**: 演示失败任务的自动重试机制。

**测试内容**:
- 创建一个会失败的任务（Execute 返回错误）
- 设置 `MaxRedo: 3`
- 演示失败任务自动重试直到重试次数耗尽

**学习要点**:
- 失败任务的执行过程
- 自动重试机制
- 重试次数管理（Redo 字段）
- 最终失败状态的判定

**预期过程**:
1. 任务执行失败（failed 状态，Redo=1）
2. 根据 `CanRetry()` 自动重新入队
3. Worker 重新获取并执行（第 2 次重试，Redo=2）
4. 再次失败并重试（第 3 次重试，Redo=3）
5. 第 3 次重试后仍失败，最终状态为 failed（Redo=3/3）

**等待时间**: 约 30-60 秒

---

### 测试 4: 健康检查和任务恢复

**目的**: 演示健康检查器检测 Worker 故障并恢复任务。

**测试内容**:
- 创建健康检查任务
- Worker 执行过程中停止更新心跳（模拟崩溃）
- 健康检查器检测到心跳超时
- 触发 `OnWorkerDead` 回调
- 任务被重新入队并执行

**学习要点**:
- Worker 心跳机制（`SetHeartbeat`）
- 健康检查器的工作原理（每 30 秒检查一次）
- 心跳超时时间配置（30 秒）
- `OnWorkerDead` 回调的处理
- 任务恢复机制

**预期过程**:
1. 创建任务并启动 Worker
2. Worker 执行期间定期更新心跳
3. 执行 10 秒后，停止更新心跳（模拟 Worker 崩溃）
4. 等待健康检查器检测到心跳超时（约 30 秒）
5. 触发 `OnWorkerDead` 回调
6. 任务被标记为失败并重新入队
7. Worker 重新获取并执行任务

**等待时间**: 约 40-60 秒

---

## 代码结构

```
example/
├── main.go         # 程序入口
├── executors.go    # 任务执行器实现
├── scenarios.go    # 测试场景实现
├── util.go         # 工具函数（数据库、Redis 连接等）
└── README.md       # 本文档
```

### 文件说明

- **main.go**: 程序入口，初始化 Worker、Manager 和 HealthChecker
- **executors.go**: 包含 4 种任务执行器实现
  - `DemoTaskExecutor`: 正常任务执行器
  - `TimeoutTaskExecutor`: 超时任务执行器（用于测试超时检测）
  - `FailTaskExecutor`: 失败任务执行器（用于测试失败重试）
  - `HealthTaskExecutor`: 健康检查任务执行器（用于测试健康检查）
- **scenarios.go**: 包含 4 个测试场景的实现
- **util.go**: 提供通用工具函数，如数据库连接、Redis 连接等

---

## 通用配置

示例使用的配置结构：

```go
// Manager 配置
managerConfig := &manager.ManagerConfig{
    KeyPrefix:      "task:example:",
    QueueBlockTime: 5 * time.Second,
}

// Worker 配置
workerConfig := &worker.WorkerConfig{
    Timeout:        10 * time.Minute,
    MaxRedo:        3,
    MaxConcurrency: 3,
    WorkerID:       "basic-worker-001",
}

// Health Checker 配置
healthConfig := &health.CheckerConfig{
    KeyPrefix:   "task:example:",
    RedisClient: redisClient,
    CheckPeriod: 30 * time.Second,
}
```

---

## 关键机制说明

### 1. 超时检测机制

`timeoutCheckRoutine` 是 Manager 中的一个后台协程，每分钟检查一次所有正在执行的任务：

- 检查任务是否超过 `Timeout` 时间
- 使用缓冲时间判断（`Timeout + 60秒`），避免误判
- 超过缓冲时间后，标记任务为 `timeout` 状态
- 超时任务的 `Redo` 字段自动 +1
- 如果 `Redo < MaxRedo`，任务会自动重新入队

**关键代码**: `manager/manager.go:429` (timeoutCheckRoutine)

---

### 2. 失败重试机制

任务执行失败后，通过 `SaveTaskResult` 保存结果时触发重试：

- 任务状态标记为 `failed`
- `Redo` 字段自动 +1
- 如果 `CanRetry()` 返回 true（即 `Redo < MaxRedo` 且状态为 `failed` 或 `timeout`），任务自动重新入队
- Worker 会重新获取并执行重试后的任务

**关键代码**: `model/entity.go:137` (CanRetry 方法), `manager/manager.go:268` (handleTaskFlowInTx)

---

### 3. 健康检查机制

健康检查器独立运行，监控 Worker 的心跳状态：

- Worker 需要定期调用 `SetHeartbeat()` 更新心跳
- 健康检查器每 30 秒检查一次所有 Worker 的心跳
- 如果心跳超过 30 秒未更新，标记 Worker 为死亡
- 触发 `OnWorkerDead` 回调，允许业务代码处理死亡的任务
- 回调中通过 `SaveTaskResult` 标记任务失败，自动触发重试

**关键代码**: `health/checker.go:217` (CheckWorkerHealth), `health/checker.go:300` (触发回调)

---

### 4. Worker 心跳管理

当前实现中，Worker 不自动管理心跳。心跳需要在外部手动控制：

```go
// 定期更新心跳
ticker := time.NewTicker(10 * time.Second)
for {
    healthChecker.SetHeartbeat(ctx, taskType, workerID, taskID)
    <-ticker.C
}
```

在实际生产环境中，建议在适配器层或专门的协程中自动管理心跳。

---

## 故障排查

### 无法连接数据库

```
Error: failed to connect database
```

**解决方法**:
1. 确保 MySQL 服务正在运行
2. 检查数据库连接配置（用户名、密码、数据库名）
3. 确保数据库已创建（demo）

### 无法连接 Redis

```
Error: failed to connect redis
```

**解决方法**:
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

### 超时任务没有立即被检测到

这是正常行为。`timeoutCheckRoutine` 每分钟检查一次，且使用缓冲时间（`Timeout + 60秒`）判断超时，因此可能需要等待 1-2 分钟才能看到超时被标记。

### 健康检查需要等待较长时间

健康检查器每 30 秒检查一次 Worker 心跳，心跳超时时间为 30 秒。因此需要等待约 30-60 秒才能看到健康检查效果。

---

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

---

## 更多资源

- [Task 包文档](../README.md) - 完整的 API 文档和设计说明
- [最佳实践](../README.md#最佳实践) - 生产环境使用建议

---

## 反馈

如果你在运行示例时遇到问题，或者有改进建议，欢迎提交 Issue！