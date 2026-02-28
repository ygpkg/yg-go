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

## 测试 SQL 示例

### 超时任务测试

以下 SQL 创建一个会触发超时检测的任务（`running` 状态，开始时间早于超时时间 + 缓冲时间）：

```sql
-- 创建一个会触发超时的任务
-- 触发条件: now > start_at + timeout + 60秒缓冲时间
-- 超时检查每分钟执行一次，且只有主节点(master)会执行
-- 
-- start_at 设置为 5 分钟前，timeout 为 30 秒
-- 此时 start_at + timeout + 60秒缓冲 ≈ 3分30秒前，已满足超时条件
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

-- 注意: 超时检测有以下前提条件：
-- 1. Manager 已启动且运行超过1分钟（定时器触发）
-- 2. 当前实例是主节点（mutex.IsMaster() 返回 true）
-- 3. 全局 Redis 缓存已初始化（redispool.InitCache 已调用）
```

### 失败任务测试

以下 SQL 创建一个失败状态的任务，会被 Worker 重新拉取并重试：

```sql
-- 创建一个失败的任务，Rdo < MaxRedo 时会被重试
INSERT INTO core_task (
    task_type, task_status, subject_id, subject_type, 
    payload, timeout, max_redo, redo,
    err_msg, created_at, updated_at
) VALUES (
    'demo_task', 'failed', 8888, 'failed_test',
    '{"message": "failed test", "user_id": 8888}', 
    30000000000, -- 30秒（纳秒）
    3, -- max_redo
    1, -- redo=1，还剩2次重试机会
    'simulated failure for testing',
    NOW(), NOW()
);
```

## 代码结构

```
example/
├── main.go      # 程序入口
├── executors.go # 任务执行器实现
├── util.go      # 工具函数
└── README.md    # 本文档
```