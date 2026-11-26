# asynctask

`asynctask` 是 yg-go 的**有状态任务中心 + 执行运行时**。它把一条后台任务当作**有状态的实体**来管理：提交后落库，执行侧通过可替换的 `Source` 取任务执行；任务可查状态、取消、重放，并保留**每一次尝试**记录；多实例下靠**租约（lease）**保证同一条任务同一时刻只被一个执行者持有，不会重复执行。

唤醒方式（纯轮询 / 轮询 + 通知器）与取任务方式（本地 DB 认领 / 远程 HTTP）都是**可替换的实现**，不是组件的前提。既能用作完整任务中心（提交 + 执行 + 管理面），也能退化成简单纯异步（只投递 + 只执行，不引管理面）。

> 本包移植自 `github.com/morehao/golib` 的 `task/gasync`（MIT，与 yg-go 同作者）。通用能力（日志、建表）改用 yg-go 自身的包，详见「来源与许可证」。
>
> 与 yg-go 既有的 Redis-Stream 版 `task/` 是**两种不同架构**，定位互补、本期并存，选型判据见文末。

## 包结构

```
asynctask                        契约内核：AsyncTask / AsyncTaskRun / AsyncTaskSpec / AsyncTaskCond /
│                                Source 接口 / Status / FailureKind / Outcome / Result / Backoff /
│                                错误哨兵（前缀 asynctask: ）/ 日志适配（logger.go）
├── wire                         远程协议（端点路径常量 + JSON 类型，客户端/服务端共用一份，只依赖根包）
├── cancelwatch                  取消通知监听器：PG LISTEN/NOTIFY 实现（abort 快路径，只负责「快」）
├── center                       远程中心服务端参考实现：Fetch / Heartbeat / Callback / Release（只依赖 manager，不含 HTTP）
├── manager                      任务中心门面：Submit/SubmitAfter/Get/GetRuns/List/ListRuns/Cancel/Replay/Stats、
│                                类型启停、租约判活、Notifier 抽象、维护循环、SchemaModels、DDL（asynctask/ddl）
├── worker                       执行运行时：每类型 1 个认领者 + N 个执行槽，执行期续租（只依赖根包）
└── source
    ├── db                       DB Source：认领/续租/保存/回收/清理 + core_async_task* 全部 SQL
    └── remote                   远程 Source：HTTP 三段 Fetch/Renew/Save（+ Release）
```

> 源 golib 里 `manager/notifier/asynq` 与 `manager/health/redis` 这两个可选子包本期**不移植**（避免引入 asynq / miniredis 依赖）。`Notifier` / `HealthReporter` / `WakeSource` / `TypeGate` 接口保留在 `manager` / `worker`，接入方如需可自带实现。

## 依赖边界

三条硬边界（`go list -deps` 可验证，`bash scripts/check-asynctask-deps.sh` 一键断言）：

- **`worker` 只依赖根包**：不 import `gorm.io/gorm` 核心、任何 `gorm.io/driver/*`、`asynctask/manager`、`asynctask/source/{db,remote}`。
- **`source/db` 只依赖根包 + gorm**：不 import `asynctask/manager`、`asynctask/worker`。
- **根包 `asynctask` 依赖树不含 gorm**：不 import `gorm.io/gorm` 核心与任何 driver（`TxFromContext` 用 `any` 抽象事务句柄）。
- `manager` 单向依赖 `source/db`；`source/db` 不反向依赖 `manager`，无循环引用。

> **ADR-0001（gorm/logger 经 logs 传递）**：yg-go 的 `logs` 包内部 import 了 `gorm.io/gorm/logger` 与 `gorm.io/gorm/utils`（仅日志辅助子包，不含 ORM 核心与 DB driver）。因此任何 import `logs` 的包（含根包 `asynctask`）在 `go list -deps` 里都会看到这两个子包。门禁只针对**有意义的边界**——ORM 核心 `gorm.io/gorm` 与任意 driver——允许 `gorm.io/gorm/logger`、`gorm.io/gorm/utils` 传递出现。「worker 不耦合 ORM/DB」的架构意图仍然成立。

## 核心概念

| 概念 | 说明 |
|---|---|
| `AsyncTask` | 有状态任务实体，对应表 `core_async_task` |
| `AsyncTaskRun` | 单次尝试记录，对应表 `core_async_task_run`；一条任务 0..N 次尝试，按 `generation`（重放轮次）分组 |
| `Source` | 取任务与回传结果的接缝：`Fetch` / `Renew` / `Release` / `Save` / `Close`；`source/db` 与 `source/remote` 是两种实现 |
| `Worker` | 执行运行时，只依赖 `Source`；每类型单点认领 → `BeforeExecute → Execute（带续租） → Save` |
| `Handler` | 业务处理器，唯一必须实现 `Execute(ctx, *asynctask.AsyncTask) error` |
| `Manager` | 任务中心门面：提交、查询、取消、重放、统计、类型启停 |

**状态机**：`pending → running → succeeded / failed / timeout / canceled / skipped`；`running` 失败可按退避回 `pending`（重试），主动让出（paused）也回 `pending`，租约过期可被他人接管；终态可 `Replay`（`generation+1`）。状态值见 `asynctask.StatusPending` 等常量，失败分类见 `asynctask.FailureKind`。

**执行身份（attempt）与失败额度（failure_count）是两件事**（设计 §9.3 Y3）：

| 列 | 语义 | 何时变化 |
|---|---|---|
| `generation` | 重放轮次 | 仅 `Replay` +1（新一轮 = 全新额度） |
| `attempt` | **单调执行序号**：一次认领 +1，一次执行对应一行 `core_async_task_run`；也是「晚到结果/晚到取消认不认得出自己」的定位依据 | 每次认领 +1；暂停后下一次认领同样 +1 |
| `failure_count` | **可重试失败次数**（准入条件 `failure_count < max_attempts`） | 可重试失败 +1、失权回收 +1、终结失败 +1；**暂停不加** |

两者分开的原因：暂停（主动让出）既要「留一条 run 记录」，又不能吃掉重试额度。
把 `attempt` 当次数用，连续暂停会被误判成连续失败；只留 `failure_count` 又会让 run 无法唯一定位。
`AsyncTask.HasRemainingAttempts()`（本次失败后还能否重试）与 `AsyncTask.Claimable()`（是否能再被认领）
都按 `failure_count` 判。

**所有权（租约）语义**：

- 认领即持有租约（`lease_until = now + LeaseTTL`），执行期由 Worker 按 `LeaseTTL/3` 续租；续租失权返回 `asynctask.ErrLeaseLost`，Worker 立即取消执行 ctx，不与接管者双跑。
- `Save` 在事务内先按 `(worker_id, generation, attempt)` 锁行确认仍持有，再跑业务钩子；失权时返回 `ErrLeaseLost` 且**不跑钩子**（状态可回滚，副作用不可回滚）。
- 停机 / 认领后无法交付用 `Release` 把租约置为已过期（**不是**续租），另一实例可立即接管。
- `manager.Start(ctx)` 起维护循环：回收「次数用尽 + 租约过期」的卡死任务、按 `Retention` 清理终态记录。

## 三种使用形态

| 形态 | 场景 | 需要的包 |
|---|---|---|
| **A 简单纯异步** | 只投递 + 只执行，不引入管理面 | 提交侧 `manager`；执行侧 `source/db` + `worker` |
| **B 完整任务中心** | 要查状态 / 取消 / 重放 / 统计 / 类型启停 | `manager`（New+Start）+ `worker` |
| **C 远程 Worker** | 执行节点只连中心、不连库 | 执行侧 `source/remote` + `worker`；中心侧 `manager` + `center` + 自建 HTTP 绑定 |

以上三种形态的**可运行完整示例**在 [`example/`](example/) 下（用 sqlite 内存库，无需外部依赖即可 `go run`）：

- [`example/a_pure_async`](example/a_pure_async/main.go) — 形态 A
- [`example/b_task_center`](example/b_task_center/main.go) — 形态 B
- [`example/c_remote`](example/c_remote/main.go) — 形态 C（含用 `net/http` + `wire` 把 center 的 4 个方法挂到路由的写法）

### 形态 A：简单纯异步

「投递 → 执行」拆成两侧，各取所需。**提交侧**只 `manager.New → Submit → Close`，不引 worker（只提交、不执行是合法用法）；**执行侧**只 `source/db` + `worker`，不构造 Manager。

```go
import (
	"context"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/manager"
	sourcedb "github.com/ygpkg/yg-go/asynctask/source/db" // 包名就叫 db，别名 sourcedb 避免与 *gorm.DB 变量重名
	"github.com/ygpkg/yg-go/asynctask/worker"
)

// ---- 提交侧（只投递）----
func Submit(ctx context.Context, db *gorm.DB, payload []byte) error {
	center, err := manager.New(&manager.Config{}, db) // 不建表：部署时执行 asynctask/ddl 的 DDL
	if err != nil {
		return err
	}
	defer center.Close()
	// Submit 三值语义：task / submitted / err（err==ErrNotify 表示已落库、通知未发出，勿重试）
	_, _, err = center.Submit(ctx, &asynctask.AsyncTaskSpec{Type: "email:send", Payload: payload})
	return err
}

// ---- 执行侧（只执行，不建 Manager）----
func RunWorkers(ctx context.Context, db *gorm.DB) error {
	src, err := sourcedb.New(sourcedb.Config{DB: db, LeaseTTL: 60 * time.Second})
	if err != nil {
		return err
	}
	// WorkerConfig.LeaseTTL 必须与 sourcedb.Config.LeaseTTL 一致
	w := worker.New(src, &worker.WorkerConfig{Concurrency: 10, LeaseTTL: 60 * time.Second})
	_ = w.Register("email:send", worker.HandlerFunc(func(ctx context.Context, t *asynctask.AsyncTask) error {
		var p EmailPayload
		if err := t.Bind(&p); err != nil {
			return worker.ErrNoRetry // 载荷反序列化失败：不重试，直接 failed
		}
		return sendEmail(ctx, p)
	}))
	if err := w.Start(ctx); err != nil { // 同步非阻塞，不用 go 关键字
		return err
	}
	defer w.Shutdown(context.Background()) // 实际用带超时的 ctx
	return nil
}
```

### 形态 B：完整任务中心

```go
func Start(ctx context.Context, db *gorm.DB, payload []byte, orderNo string) (*asynctask.AsyncTask, bool, error) {
	center, err := manager.New(&manager.Config{LeaseTTL: 60 * time.Second}, db) // 不建表：部署时执行 asynctask/ddl 的 DDL
	if err != nil {
		return nil, false, err
	}
	defer center.Close()

	// 必须显式启动维护循环：否则「次数用尽后崩溃」的任务会永久停在 running，终态记录也不会按 Retention 清理
	if err := center.Start(ctx); err != nil {
		return nil, false, err
	}

	w := worker.New(center.Source(), &worker.WorkerConfig{
		Concurrency: 10,
		LeaseTTL:    60 * time.Second, // 必须与 manager.Config.LeaseTTL 一致
	}) // 复用 manager 内部的 DB Source
	_ = w.Register("invoice:issue", &InvoiceHandler{db: db}) // 需实现 Execute(ctx, *asynctask.AsyncTask) error
	if err := w.Start(ctx); err != nil {
		return nil, false, err
	}
	defer w.Shutdown(context.Background())

	// UniqKey 命中时 submitted=false，返回既有 task（幂等投递）
	return center.Submit(ctx, &asynctask.AsyncTaskSpec{
		Type: "invoice:issue", Payload: payload,
		BizType: "order", BizID: orderNo, UniqKey: "invoice:issue:" + orderNo,
	})
}
```

### 形态 C：远程 Worker（执行节点只连中心、不连库）

执行节点用 `source/remote`，Worker 代码与形态 A 完全一致，只是换了 `Source`：

```go
import (
	"github.com/ygpkg/yg-go/asynctask/source/remote"
	"github.com/ygpkg/yg-go/asynctask/worker"
)

func RunRemoteWorker(ctx context.Context) error {
	src, err := remote.New(remote.Config{BaseURL: "https://center.example.com/asynctask", WorkerID: "w-1"})
	if err != nil {
		return err
	}
	w := worker.New(src, &worker.WorkerConfig{Concurrency: 10, LeaseTTL: 60 * time.Second})
	_ = w.Register("email:send", worker.HandlerFunc(func(ctx context.Context, t *asynctask.AsyncTask) error {
		var p EmailPayload
		if err := t.Bind(&p); err != nil {
			return worker.ErrNoRetry
		}
		return sendEmail(ctx, p)
	}))
	if err := w.Start(ctx); err != nil {
		return err
	}
	defer w.Shutdown(context.Background())
	return nil
}
```

> 同一份 `Handler` + `Worker` 可在本地 / 远程两种 `Source` 上运行，结论一致。远程形态下钩子**拿不到事务**（`asynctask.TxFromContext` 返回 `nil`），业务钩子不得假设能拿到 tx。

**中心侧（服务端）** —— `manager`（落库 + 维护循环）+ `center`（协议逻辑）+ 自建 HTTP 绑定：

```go
import (
	"github.com/ygpkg/yg-go/asynctask/center"
	"github.com/ygpkg/yg-go/asynctask/manager"
)

mgr, err := manager.New(&manager.Config{LeaseTTL: 60 * time.Second}, db)
if err != nil {
	return err
}
if err := mgr.Start(ctx); err != nil { // 远端 worker 崩溃后靠维护循环判死
	return err
}
defer mgr.Close()

c, err := center.New(mgr)
if err != nil {
	return err
}
// yg-go 本期不提供 ginasync.RegisterCenter；用 gin/httptools 自行把 center 的 4 个方法挂到路由。
// 端点路径一律取 wire 包常量：wire.PathFetch / PathHeartbeat / PathCallback / PathRelease。
// 例如（伪代码）：
//   r.POST(wire.PathFetch,     func(c *gin.Context) { req := wire.FetchRequest{}; ... ; resp,_ := center.Fetch(c, req); c.JSON(200, resp) })
//   r.POST(wire.PathHeartbeat, ...)  r.POST(wire.PathCallback, ...)  r.POST(wire.PathRelease, ...)
```

> **center 只依赖 manager，不含 HTTP。** 完整可运行的路由挂载写法（`net/http` + `wire`）见 [`example/c_remote`](example/c_remote/main.go) 的 `mountCenter`。

#### 远程中心的 4 个端点（`source/remote` 的对接契约）

| 常量 | 路径 | 请求体 | 响应 |
|---|---|---|---|
| `wire.PathFetch` | `POST {base}/GetPendingAsyncTask` | `{"task_type","worker_id"}` | `200` + `FetchResponse{available, asyncTask}`；无任务 `available:false` |
| `wire.PathHeartbeat` | `POST {base}/Heartbeat` | `{"task_id","worker_id"}` | `200`/`204` 仍持有；非归属 `ErrLeaseLost` |
| `wire.PathCallback` | `POST {base}/Callback` | `{"task_id","worker_id","status","result","error"}` | `200`/`204`；失败客户端重试（默认 3 次后 `ErrCallbackLost`），已终态幂等 ACK |
| `wire.PathRelease` | `POST {base}/Release` | `{"task_id","worker_id"}` | `200`/`204`；`404`/`405` 视为中心未实现，按无操作处理 |

`wire.Task` 字段：`id/type/priority/payload/attempt/max_attempts/timeout_ms/request_id/trace_id`。中心侧要做到：同一任务同一时刻只派给一个 `worker_id`、`Heartbeat` 校验归属、`Callback` 只接受当前归属者的结果。

## 关键 API 速览

### 根包 `asynctask`

- `AsyncTask`：`Bind(v)` 反序列化 payload；`IsStuck(now)` / `CanCancel()` / `HasRemainingAttempts()`（按失败额度） / `Claimable()`。
- 暂停/取消/结算的契约类型：`OutcomePaused` + `Result.PauseUntil` / `Result.PauseReason`；
  `CancelSignal`（`Encode` / `DecodeCancelSignal` / `Match`）与可选接口 `CancelWatcher`；
  可选扩展接口 `StatusReader`（取消补查）与 `LeaseTxn`（持租约事务，给下游同事务结算）；`RunReclaimHook`（失权回收结算钩子）。
- `AsyncTaskSpec`：`Type`（必填）、`Payload`、`Priority`、`MaxAttempts`、`Timeout`、`RunAt`/`Delay`、`UniqKey`、`BizType`/`BizID`、`ChainID`/`Step`、`Labels`、`RequestID`。
- `AsyncTaskCond`（列表查询）：`Page`/`PageSize`/`Type`/`Status`/`BizType`/`BizID`/`ChainID`/`Labels`/`CreatedAfter`/`CreatedBefore`/`OrderBy`。
- `Source` 接口、`Result`/`Outcome`、`EnsureFunc`、`WithTx`/`TxFromContext`、`Backoff`、`Logger`/`DefaultLogger`、错误哨兵（前缀 `asynctask:`，如 `ErrLeaseLost` / `ErrNotify` / `ErrNoAsyncTask` / `ErrInvalidConfig` / `ErrPayloadTooLarge`）。

### `source/db`

包路径 `github.com/ygpkg/yg-go/asynctask/source/db`，**包名就叫 `db`**（与 `*gorm.DB` 变量重名时会冲突，引用时通常别名成 `sourcedb`）：

```go
import sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"

func New(cfg Config) (*Source, error) // Config{DB *gorm.DB, LeaseTTL, Backoff, WorkerID, FailFastChain, MaxResultBytes, ReclaimHook}
// 可选扩展（断言即用）：asynctask.StatusReader.TaskStatus / asynctask.LeaseTxn.WithLeaseTx
func (s *Source) Store() Store        // 只读查询（GetAsyncTask/ListAsyncTask/GetActiveByUniqKey/GetLatestBySubject/StatsByStatus/...）
// Source 仅实现 asynctask.Source：Fetch / Renew / Release / Save / Close；不含 Submit（提交走 manager）
```

### `manager`

- `New(cfg *Config, db *gorm.DB, opts ...Option) (*Manager, error)`；`Start(ctx)`（维护循环）/ `Close`；`Submit` / `SubmitAfter`（步骤链）/ `Get` / `GetRuns` / `List` / `ListRuns` / `Cancel` / `Replay` / `Stats` / `StatsCond` / `GetByUniqKey` / `GetLatestBySubject` / `Mode` / `Store` / `Source` / `FetchFor`。
- 类型级启停：`Disable(ctx, type)` / `Enable(ctx, type)` / `IsTypeEnabled` / `TypeGate(ttl)`。
- 存活判定：`Registry()`（基于租约推导，零外部依赖）。
- 实体清单：`SchemaModels() []any`（只返回实体，不执行任何 DDL）。
- 建表：**本包不建表**，DDL 见 `asynctask/ddl`（`Postgres()` / `MySQL()` / `SQLite()`，幂等可重放）。
- 选项：`WithNotifier` / `WithBackoff` / `WithBatchSize` / `WithLeaseTTL` / `WithRetention` / `WithPollInterval`（后者**已废弃不生效**，轮询由 `worker.WorkerConfig.PollInterval` 决定）。
- `Config` 关键字段：`LeaseTTL`(默认 60s) / `ReaperInterval`(30s) / `CleanupInterval`(1h) / `DefaultMaxAttempts`(3) / `DefaultTimeout`(30s) / `MaxPayloadBytes`(4096) / `MaxResultBytes`(4096，结果快照截断上限，超限按前缀截断不报错) / `Retention`(30d) / `FailFastChain` / `Backoff` / `BatchSize`(50) / `NotifierQueuePrefix`。

### `worker`

- `New(src asynctask.Source, cfg *WorkerConfig) *Worker`；`Register(type, handler, opts...)`（必须在 `Start` 前）；`Start(ctx) error`（同步非阻塞）/ `Shutdown(ctx) error` / `Run(ctx) error`（阻塞兼容层）。
- `Handler` / `HandlerFunc`；`HandlerOption`：`WithHandlerTimeout` / `WithHandlerConcurrency`。
- 控制流哨兵：`worker.ErrSkip`（不执行直接 skipped）/ `worker.ErrNoRetry`（立即 failed，不消耗剩余次数）/ `worker.ErrPause`（主动让出，不消耗失败额度）。
- 暂停：`worker.Pause(retryAfter, reason)` 返回 `*PauseError`；handler 可实现可选接口 `worker.PauseLifecycle`（`OnPause`）在**结算事务内**结算阶段预留额。
- `WorkerConfig`：`Concurrency` / `ShutdownTimeout` / `WorkerID` / `Logger`（`asynctask.Logger`，默认 `asynctask.DefaultLogger()`）/ `EventHandler` / `LeaseTTL` / `PollInterval` / `FallbackPollInterval` / `WakeSource` / `HealthReporter` / `HealthReportInterval` / `TypeGate` / `TypeGateTTL` / `CancelWatcher` / `CancelRecheckInterval`。

### `cancelwatch`（取消快路径）

```go
import "github.com/ygpkg/yg-go/asynctask/cancelwatch"

watcher, err := cancelwatch.New(cancelwatch.Config{
    DSN:     "<PG DSN>",                  // LISTEN 需要一条独占连接
    Channel: "asynctask_cancel",          // 与 manager.Config.CancelNotifyChannel 一致
})
// WorkerConfig.CancelWatcher = watcher；退出时 watcher.Close()
```

使用注意：

- `Watch` **首次调用同步建连 + LISTEN**：DSN 配错（指向 MySQL 端口、通道名非法、库不可达）会在
  `Watch` 返回错误，而不是静默地每 3s 重试一次。之后断线仍由内部循环重连。
- `Close` 只停监听，**不关闭信号通道**：消费方（worker）靠自己的 `ctx` 退出。因此可以把
  `watcher.Close()` 和 `worker.Shutdown()` 按任意顺序调用，但不要指望 `Close` 去唤醒消费循环。
- 一个 watcher 对应一个执行期：监听循环跟随首次 `Watch` 的 `ctx`，ctx 结束即退出且不会再重建。
  重复 `Start`/`Stop` worker 时需要重新构造 watcher（`worker.Start` 对已停机的实例本身返回
  `ErrWorkerStopped`）。取消的正确性在 watcher 缺席时由「登记后复查 + 定时补查」兜底。

**取消语义**（设计 §9.1）：`Manager.Cancel` 在状态翻转事务内 `pg_notify`，
worker 的常驻监听器按 `(task_id, generation, attempt)` 查「本地活跃执行表」并取消匹配执行；
登记后立刻复查 DB（注册竞态）、定时补查（`CancelRecheckInterval`，默认 30s）兜住漏通知，
续租失权仍是最外层兜底。唤醒（`WakeSource`/`Notifier`）是另一条通道，不要复用。

### `wire` / `center` / `source/remote`

- `wire`：路径常量 `PathFetch` / `PathHeartbeat` / `PathCallback` / `PathRelease`；类型 `Task` / `FetchRequest` / `FetchResponse` / `HeartbeatRequest` / `CallbackRequest` / `ReleaseRequest`；`FromAsyncTask` / `ParseOutcome`；只依赖根包。
- `center`：`New(*manager.Manager) (*Center, error)`；`Fetch` / `Heartbeat` / `Callback` / `Release`（与 `source/remote` 四端点一一对应）；**只依赖 manager，不含 HTTP**。
- `source/remote`：`New(Config) (*Source, error)`，`Config{BaseURL, WorkerID, HTTPClient, Timeout, CallbackMaxRetry, CallbackRetryDelays, RenewMaxFailures}`；`NoAsyncTaskBackoff`。

## 数据表

**本包不建表**（`New` 不执行任何 DDL）。表结构真源是可评审的 SQL 文件，由应用的统一迁移流程执行一次：

| 方言 | 文件 | 获取方式 |
|---|---|---|
| PostgreSQL 12+ | `asynctask/ddl/postgres.sql` | `ddl.Postgres()` |
| MySQL 8.0.1+ | `asynctask/ddl/mysql.sql` | `ddl.MySQL()`（含生成列 `uniq_key_active`） |
| SQLite 3（仅测试） | `asynctask/ddl/sqlite.sql` | `ddl.SQLite()`（与 PG 同语义：部分唯一索引） |

三份 DDL 都幂等、可重放；`ddl.Apply(db)` 逐条执行（仅供测试/示例，生产请走迁移流程）。
`ddl/` 下的对账测试会把 `SchemaModels()` 的列名/索引名与三份 DDL 双向比对，任何一侧多/少一个都红。

| 表名 | 说明 |
|---|---|
| `core_async_task` | 任务实体（id、task_type、status、priority、payload、uniq_key、biz_type/biz_id、**company_id/subject_kind/subject_id**、chain_id/step/prev_step、labels、generation/**attempt（执行序号）**/**failure_count（失败额度）**/max_attempts、next_run_at/lease_until、各时间与 request id） |
| `core_async_task_run` | 单次尝试记录（id、async_task_id、generation/attempt、status、failure_kind、起止、耗时、result、error、request id）；`status='paused'` 表示这次执行是主动让出 |
| `core_async_task_type` | 任务类型开关（id、task_type 唯一、enabled；**无行=启用**，fail-open） |

幂等键的唯一性范围是**活跃期**而不是整行生命期：PG/SQLite 用部分唯一索引
`uk_core_async_task_uniq_active (uniq_key) WHERE status IN ('pending','running')`，
MySQL 用 STORED 生成列 `uniq_key_active` + 唯一键模拟。因此**终态（failed/succeeded/…）之后
用同一个 `UniqKey` 可以重建新任务**；活跃期内重复提交仍命中既有任务。

表名常量见 `source/db` 的 `CoreAsyncTaskTableName` / `CoreAsyncTaskRunTableName` / `CoreAsyncTaskTypeTableName`。

## 接入要点

- **建表/升级**：`manager.New` 不会建表。新库请执行 `ddl.Postgres()` / `ddl.MySQL()`；
  本改造之前（v1.27.5 及更早）建过表的库请执行设计文档 §9 的升级片段（加 3 列 + 2 索引、删旧全局唯一索引、建 `uk_core_async_task_uniq_active`）。
- **v1.27.12 起新增列 `failure_count`**（执行身份与预算：失败重试计数，与 `attempt` 执行序号分离）。
  新库直接跑三份 DDL 即可（列已内含）；**已有库**按方言补列：

  ```sql
  -- PostgreSQL 12+：ddl/postgres.sql 里已带这句（幂等，重放安全），无需手工执行
  ALTER TABLE core_async_task ADD COLUMN IF NOT EXISTS failure_count integer NOT NULL DEFAULT 0;

  -- MySQL 8 / SQLite：没有 ADD COLUMN IF NOT EXISTS，只能一次性执行（列已存在时会报错，属预期）
  ALTER TABLE core_async_task ADD COLUMN failure_count int NOT NULL DEFAULT 0;      -- MySQL 8
  ALTER TABLE core_async_task ADD COLUMN failure_count integer NOT NULL DEFAULT 0;  -- SQLite 3

  -- 一次性回填（三种方言都要，且只执行一次）：存量行按旧口径 = attempt 回填，
  -- 否则老任务的失败额度会被放宽成 max_attempts 次全新重试。
  -- 注意：不能写进可重放的 DDL 文件，重跑会覆盖新语义下真实的 failure_count。
  UPDATE core_async_task SET failure_count = attempt WHERE failure_count = 0 AND attempt > 0;
  ```

  注意：`attempt` 的语义同时收窄为「单调执行序号」（不再是重试次数）。存量 `running` 行回填后
  行为与旧版一致；不回填则它们会多出若干次重试机会。
- **暂停（主动让出）**：handler 返回 `worker.Pause(retryAfter, reason)`；结算在同一事务内完成
  （核验租约 → 收尾 run（finishRun）→ 结算钩子 → 任务回 pending；钩子失败则整个事务回滚，
  任务仍 running、租约到期后由回收器按预留额结算），`failure_count` 不变、`attempt` 不归零。
  想在同事务里结算自己的预算/断点，实现 `worker.PauseLifecycle`。
- **自定义 `asynctask.Source` 必须在 `Save` 里处理 `asynctask.OutcomePaused`**（本地 `source/db`
  已处理；远程形态由 `center` 转发，无需接入方处理）。沿用旧契约「default → failed」的实现会把
  主动让出记成一次失败，吃掉重试额度。
- **持租约事务**：`src.(asynctask.LeaseTxn).WithLeaseTx(ctx, task, fn)`——失权返回 `ErrLeaseLost` 且不调用 `fn`；
  `fn` 内用 `asynctask.TxFromContext(ctx)` 取 `*gorm.DB`。装饰过的 Source 需要自己转发该接口。
- **取消通知**：`manager.New(..., manager.WithCancelNotifyChannel("asynctask_cancel"))` +
  `worker.WorkerConfig.CancelWatcher`（见上）。不配则没有通知通道，取消由「登记后复查 +
  续租失权 / 定时补查」发现——比通知慢（一个续租周期量级），但不丢；注意 `recheckOnRegister`
  是无条件执行的，Source 实现 `StatusReader` 时每次执行登记后会多一次按主键状态查询。
- **注入写主库**：`manager.New` / `source/db.New` 传入的 `*gorm.DB` 必须是**写主库**；读写分离下注入从库会让认领读到延迟数据、幂等回查读不到刚插入的行。
- **handler 必须幂等**：租约过期被接管、停机超时被接管都会重跑；用 `AsyncTask.ID` + `Generation` + `Attempt` 做幂等键。
- **handler 必须响应 `ctx.Done()`**：超时/取消通过取消 ctx 表达；忽略它会在超时后继续跑完。
- **`Submit` 三值语义**：`err == nil` 成功；`err == asynctask.ErrNotify` 已落库但通知没发出（**勿重试**，兜底轮询接手）；`submitted` 区分是否新建（幂等键命中**活跃行**时 false、返回既有任务；终态行不算命中）。
- **一等维度**：租户与业务主体用 `AsyncTaskSpec.CompanyID / SubjectKind / SubjectID`（对应独立列与索引），不要再塞进 `Labels`；查列表/统计用 `AsyncTaskCond.CompanyID`（`*uint64`，0 是合法租户）/ `SubjectKind` / `SubjectID` / `ActiveOnly` / `AfterCreatedAt`+`AfterID`（keyset 分页）。
- **两处 LeaseTTL 必须一致**：`worker.WorkerConfig.LeaseTTL` 与 `manager.Config.LeaseTTL` / `source/db.Config.LeaseTTL` 是同一个租约时长，Worker 侧小了会被误判失权，大了接管会变慢。
- **记得 `manager.Start(ctx)`**：管理面进程要回收/清理就必须启动维护循环；只提交不管理的进程可以只 `New` + `Close`。
- **不用 gormdao 软删除**：三张表无软删除语义，用 `gormdao` 接入时传 `WithoutSoftDelete()`。

## 与旧 `task/`（Redis Stream 版）的选型判据

| 维度 | 旧 `task/`（Redis Stream） | 新 `asynctask/`（DB 租约） |
|---|---|---|
| 队列载体 | Redis Stream | 数据库表 `core_async_task*` |
| 互斥/接管 | 心跳 + Redis | DB 行租约 + `FOR UPDATE SKIP LOCKED` |
| 任务状态 | pending/running/success/failed/canceled/timeout | 同集合 + skipped + generation 重放 |
| 每次尝试记录 | 无独立表 | `core_async_task_run` |
| 重放/步骤链/类型启停 | 部分（Step/AppGroup） | 完整（Replay/SubmitAfter/TypeGate） |
| 多租户 | 内置 CompanyID/Uin | 一等维度 `company_id` / `subject_kind` / `subject_id`（带索引） |
| Redis 依赖 | **必需** | **不需要** |

**怎么选**：

- 已有 Redis、要低延迟高吞吐的流式队列、任务以「批量/短任务」为主 → 旧 `task/`。
- **无 Redis / 需要单条任务的状态机与重放 / 需要弱耦合多租户 / 要在数据库里直接查任务** → `asynctask/`。
- 两者**并存**，本期不做旧 `task/` 存量迁移；未来若收敛为一套另立迁移方案。

## 来源与许可证

本包移植自 [`github.com/morehao/golib`](https://github.com/morehao/golib) 的 `task/gasync`，原作者 morehao。

- 源项目许可证：**MIT**；本仓 yg-go 为 **GPL-3.0**。MIT → GPL-3.0 兼容，可移植。
- 移植改动：日志改用 yg-go `logs`（经包内 `asynctask.Logger` 适配，见 `logger.go`）；日志 key 常量改为包内常量（值仍为 `task.type`/`task.id`）；建表改为包内 `asynctask/ddl` 的三方言 SQL（不执行 DDL、不再 AutoMigrate）；不引入 asynq / miniredis / Redis 心跳子包。
- 表名沿用 `core_async_task` / `core_async_task_run` / `core_async_task_type`（与旧 `core_task` 语义独立、不冲突）。
