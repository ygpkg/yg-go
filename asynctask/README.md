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

**状态机**：`pending → running → succeeded / failed / timeout / canceled / skipped`；`running` 失败可按退避回 `pending`（重试），租约过期可被他人接管；终态可 `Replay`（`generation+1`）。状态值见 `asynctask.StatusPending` 等常量，失败分类见 `asynctask.FailureKind`。

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

- `AsyncTask`：`Bind(v)` 反序列化 payload；`IsStuck(now)` / `CanCancel()` / `HasRemainingAttempts()`。
- `AsyncTaskSpec`：`Type`（必填）、`Payload`、`Priority`、`MaxAttempts`、`Timeout`、`RunAt`/`Delay`、`UniqKey`、`BizType`/`BizID`、`ChainID`/`Step`、`Labels`、`RequestID`。
- `AsyncTaskCond`（列表查询）：`Page`/`PageSize`/`Type`/`Status`/`BizType`/`BizID`/`ChainID`/`Labels`/`CreatedAfter`/`CreatedBefore`/`OrderBy`。
- `Source` 接口、`Result`/`Outcome`、`EnsureFunc`、`WithTx`/`TxFromContext`、`Backoff`、`Logger`/`DefaultLogger`、错误哨兵（前缀 `asynctask:`，如 `ErrLeaseLost` / `ErrNotify` / `ErrNoAsyncTask` / `ErrInvalidConfig` / `ErrPayloadTooLarge`）。

### `source/db`

包路径 `github.com/ygpkg/yg-go/asynctask/source/db`，**包名就叫 `db`**（与 `*gorm.DB` 变量重名时会冲突，引用时通常别名成 `sourcedb`）：

```go
import sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"

func New(cfg Config) (*Source, error) // Config{DB *gorm.DB, LeaseTTL, Backoff, WorkerID, FailFastChain, MaxResultBytes}
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
- 控制流哨兵：`worker.ErrSkip`（不执行直接 skipped）/ `worker.ErrNoRetry`（立即 failed，不消耗剩余次数）。
- `WorkerConfig`：`Concurrency` / `ShutdownTimeout` / `WorkerID` / `Logger`（`asynctask.Logger`，默认 `asynctask.DefaultLogger()`）/ `EventHandler` / `LeaseTTL` / `PollInterval` / `FallbackPollInterval` / `WakeSource` / `HealthReporter` / `HealthReportInterval` / `TypeGate` / `TypeGateTTL`。

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
| `core_async_task` | 任务实体（id、task_type、status、priority、payload、uniq_key、biz_type/biz_id、**company_id/subject_kind/subject_id**、chain_id/step/prev_step、labels、generation/attempt/max_attempts、next_run_at/lease_until、各时间与 request id） |
| `core_async_task_run` | 单次尝试记录（id、async_task_id、generation/attempt、status、failure_kind、起止、耗时、result、error、request id） |
| `core_async_task_type` | 任务类型开关（id、task_type 唯一、enabled；**无行=启用**，fail-open） |

幂等键的唯一性范围是**活跃期**而不是整行生命期：PG/SQLite 用部分唯一索引
`uk_core_async_task_uniq_active (uniq_key) WHERE status IN ('pending','running')`，
MySQL 用 STORED 生成列 `uniq_key_active` + 唯一键模拟。因此**终态（failed/succeeded/…）之后
用同一个 `UniqKey` 可以重建新任务**；活跃期内重复提交仍命中既有任务。

表名常量见 `source/db` 的 `CoreAsyncTaskTableName` / `CoreAsyncTaskRunTableName` / `CoreAsyncTaskTypeTableName`。

## 接入要点

- **建表/升级**：`manager.New` 不会建表。新库请执行 `ddl.Postgres()` / `ddl.MySQL()`；
  本改造之前（v1.27.5 及更早）建过表的库请执行设计文档 §9 的升级片段（加 3 列 + 2 索引、删旧全局唯一索引、建 `uk_core_async_task_uniq_active`）。
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
