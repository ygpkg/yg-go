# Task 包使用示例

本示例展示如何使用 Task 包启动一个持续运行的 Worker 服务。

## 快速开始

### 前置条件

确保 MySQL 和 Redis 服务正在运行：

```sql
-- MySQL 创建数据库
CREATE DATABASE demo;
```

### 运行示例

```bash
cd example
go run .
```

## 核心组件

示例启动了三个核心组件：

1. **Manager**: 任务管理器，负责队列同步和超时检测
2. **Worker**: 任务执行器，从队列获取任务并执行
3. **HealthChecker**: 健康检查器，监控 Worker 存活状态

## 任务执行器

实现 `worker.TaskExecutor` 接口：

```go
type TaskExecutor interface {
    Execute(ctx context.Context) error
    GetResult() string
    SetResult(result string)
    OnSuccess(ctx context.Context) error
    OnFailure(ctx context.Context) error
}
```

注册执行器：

```go
w.RegisterExecutor("demo_task", func(payload string) (worker.TaskExecutor, error) {
    return NewDemoTaskExecutor(payload)
})
```

## 验证 queueSyncRoutine

`queueSyncRoutine` 是队列同步协程，负责确保队列中有足够消息触发 Worker 消费。

### 工作原理

- **同步间隔**: 每 10 秒执行一次（配置项 `QueueSyncInterval`）
- **执行条件**: 只有 Master 节点才会执行队列同步
- **核心逻辑**: 检查数据库中待处理任务数量，如有待处理任务则向 Redis 队列推送消息

### 验证步骤

1. **准备环境**: 启动 MySQL 和 Redis 服务，创建 `demo` 数据库
2. **运行示例**: `cd task/example && go run .`
3. **确认主节点**: 观察启动日志 `主节点状态: true`
4. **观察同步日志**: 每 10 秒会输出队列同步日志：
   ```
   [task] synced queue for taskType: demo_task, pending tasks: X
   ```
5. **手动创建任务验证**:
   ```sql
   INSERT INTO core_task (
       task_type, task_status, subject_id, subject_type, 
       payload, timeout, max_redo, created_at, updated_at
   ) VALUES (
       'demo_task', 'pending', 100, 'sync_test',
       '{"message": "queue sync test", "user_id": 100}', 
       30000000000, 3, NOW(), NOW()
   );
   ```
   创建后观察日志，确认队列同步已处理新任务。

## 验证 timeoutCheckRoutine

`timeoutCheckRoutine` 是超时检查协程，负责检测并处理超时的任务。

### 工作原理

- **检查间隔**: 每分钟执行一次
- **执行条件**: 只有 Master 节点才会执行超时检查
- **核心逻辑**: 检查 `running` 状态且已超时的任务，将其状态改为 `timeout`

### 验证步骤

1. **准备环境**: 启动 MySQL 和 Redis 服务，创建 `demo` 数据库
2. **运行示例**: `cd task/example && go run .`
3. **确认主节点**: 观察启动日志 `主节点状态: true`
4. **创建超时任务**:
   ```sql
   INSERT INTO core_task (
       task_type, task_status, subject_id, subject_type, 
       payload, timeout, max_redo, created_at, updated_at,
       start_at, worker_id
   ) VALUES (
       'demo_task', 'running', 9999, 'timeout_test',
       '{"message": "timeout test", "user_id": 9999}', 
       30000000000, -- 30秒（纳秒）
       3, 
       NOW(), NOW(),
       DATE_SUB(NOW(), INTERVAL 5 MINUTE), -- 5分钟前开始，确保超时
       'test-worker-001'
   );
   ```
5. **观察日志**: 等待约 1 分钟后，观察超时检测日志：
   ```
   [task] timeout check running, isMaster: true
   [task] timeout check completed successfully
   ```
6. **验证结果**: 查询数据库确认任务状态已变为 `timeout`
   ```sql
   SELECT id, task_status FROM core_task WHERE subject_id = 9999;
   ```

## 验证健康检查机制

`health.Checker` 是健康检查器，负责监控 Worker 存活状态并处理死亡 Worker 的任务。

### 工作原理

#### 心跳存储结构
- **存储位置**: Redis Hash，键格式为 `{KeyPrefix}task_heartbeat:{taskType}`
- **数据格式**: `timestamp-taskID`（时间戳-任务ID）
- **示例**: Worker `worker-001` 执行任务 `123` 时，心跳值为 `1709012345-123`

#### 检查流程
1. **检查周期**: 每 30 秒执行一次（`DefaultCheckPeriod`）
2. **执行条件**: 只有 Master 节点才会执行健康检查
3. **超时判定**: 当前时间戳 - 心跳时间戳 > 30 秒（`HeartbeatTimeout`）
4. **死亡处理**: 调用 `OnWorkerDead` 回调，将死亡 Worker 的任务标记为失败

### 验证健康检查

此方式用于验证健康检查器的基本功能。

1. **准备环境**: 启动 MySQL 和 Redis 服务，创建 `demo` 数据库
2. **运行示例**: `cd task/example && go run .`
3. **确认主节点**: 观察启动日志 `主节点状态: true`
4. **创建测试任务**:
   ```sql
   INSERT INTO core_task (
       task_type, task_status, subject_id, subject_type, 
       payload, timeout, max_redo, created_at, updated_at
   ) VALUES (
       'demo_task', 'running', 8888, 'health_test',
       '{"message": "health check test"}', 
       30000000000, 3, NOW(), NOW()
   );
   SELECT id FROM core_task WHERE subject_id = 8888;
   ```
   > 记录返回的任务 ID（假设为 123），后续步骤需要使用
   
5. **手动插入过期心跳**（将 `{TASK_ID}` 替换为步骤4返回的真实任务ID）:
   ```bash
   redis-cli HSET task:example:task_heartbeat:demo_task worker-002 "1709012345-{TASK_ID}"
   # 示例：redis-cli HSET task:example:task_heartbeat:demo_task worker-002 "1709012345-123"
   ```
   > 注意：时间戳 `1709012345` 是一个过期的 Unix 时间戳

6. **观察日志**: 等待最多 30 秒，观察健康检查日志：
   ```
   [task] worker expired: worker-002, taskType: demo_task, last heartbeat: 1709012345
   ! 发现死亡 Worker: worker-002, 任务ID: 123
   ```
7. **验证任务状态**: 检查任务是否被标记为失败
   ```sql
   SELECT id, task_status, err_msg FROM core_task WHERE subject_id = 8888;
   ```
   预期结果：`task_status` 为 `failed`，`err_msg` 为 `worker heartbeat timeout`

### 注意事项

1. **Master 职责**: 健康检查只在 Master 节点执行，确保不会重复处理
2. **心跳超时**: 默认 30 秒，可根据业务需求调整 `HeartbeatTimeout` 常量
3. **回调处理**: `OnWorkerDead` 回调用于处理死亡 Worker 的任务（如标记失败、重新入队等）
4. **心跳清理**: 正常停止 Worker 时应调用 `DeleteHeartbeat` 清理心跳记录

## 项目结构

```
example/
├── main.go      # 程序入口
├── executors.go # 任务执行器实现
├── util.go      # 工具函数
└── README.md    # 本文档
```