# dbtools v1 → v2 迁移方案

## 架构变更

v2 将 v1 中硬编码的驱动 `switch` 替换为**插件注册机制**。每个驱动通过独立子包的 `init()` 自注册，实现按需编译、解耦依赖。

```
v1: InitDBConn() 内部 switch scheme → 直接 import 4 个 GORM driver
v2: mysqldrv/pgdrv/sqlitedrv/clickhousedrv 各自 Register() → Open() 按 scheme 查找
```

## API 对照

### 完全兼容（签名、语义相同）

| v1 函数 | v2 函数 |
|---|---|
| `InitModel` | `InitModel` |
| `DoInitModels` | `DoInitModels` |
| `DoInitModelsWithDB` | `DoInitModelsWithDB` |
| `InsertOrUpdate` | `InsertOrUpdate` |
| `InitDBConn` | `InitDBConn`（实现改为插件查找） |
| `RegistryDB` | `RegistryDB` |
| `DB` | `DB` |
| `DBExists` | `DBExists` |
| `Std` / `Core` / `Account` | `Std` / `Core` / `Account` |
| `InitModelFunc` / `InitModelWithDBFunc` | 同 |

### 重命名 / 拼写修正

| v1 | v2 | 说明 |
|---|---|---|
| `InitMutilDBConn` | `InitMultiDBConn` | 修正拼写错误 Mutil → Multi |

### v1 已移除（v2 有替代）

| v1 | v2 替代 | 说明 |
|---|---|---|
| `InitMySQL` / `InitMutilMySQL` | `InitDBConn` + `mysqldrv` 注册 | 不再需要 MySQL 专用入口 |
| `NormalizeMySQL` | `mysqldrv.normalizeMySQL`（未导出） | 内部实现，外部无需调用 |
| `database.go` | `v2/helpers.go` | 完全重复，已删除 |
| `datasource.go` | `v2/manager.go` + `v2/registry.go` | 已删除 |
| `mysql.go` | `v2/mysqldrv` | 已删除 |
| `clickhousetool/` | `v2/clickhousedrv` | 已删除 |
| `sqlitetool/` | `v2/sqlitedrv` | 已删除 |
| `pgtool/` | `v2/pgdrv` + `InitPostgresWithPool` | 已删除 |

### v2 新增

| 函数 | 说明 |
|---|---|
| `InitPostgresWithPool` | 独立 Postgres 连接池配置 |
| `Register` / `Open` / `DriverFactory` | 插件注册体系 |

## 迁移步骤

### 1. 修改 import 路径

```go
// before
"github.com/ygpkg/yg-go/dbtools"

// after
dbtools "github.com/ygpkg/yg-go/dbtools/v2"
```

### 2. 确保驱动注册

在入口文件或测试文件添加驱动导入：

```go
import _ "github.com/ygpkg/yg-go/dbtools/v2/mysqldrv"
// 按需: pgdrv / sqlitedrv / clickhousedrv
```

### 3. 替换已移除函数

```go
// before
dbtools.InitMutilMySQL(conns)

// after
dbtools.InitMultiDBConn(conns)  // URL 格式: mysql://user:pass@host/db
```

### 4. 调用方清单（v1 残留）

| 文件 | 使用的 v1 函数 | 迁移操作 |
|---|---|---|
| `task/manager/init.go` | `InitModel` | 改 import |
| `apis/exportjob/export_job.go` | `Core()` | 改 import |
| `apis/exportjob/job_update.go` | `Core()` | 改 import |
| `apis/exportjob/job_monitor.go` | `Core()` | 改 import |
| `prompt/init.go` | `InitModel`, `Core()` | 改 import |
| `pool/svrpool/pool_test.go` | `InitMutilMySQL`, `Core()` | 改 import + 替换函数 |
| `notify/sms/verify_code.go` | `Core()` | 改 import |

## 子包说明

以下子包为独立工具，v2 中无对应替代，保留：

- `redispool` — Redis 连接池、缓存封装、分布式锁
- `esquery` — Elasticsearch DSL 构建器
- `estool` — Elasticsearch 客户端封装

以下子包已被 v2 驱动包替代并删除：

- ~~`clickhousetool`~~ → `v2/clickhousedrv`
- ~~`sqlitetool`~~ → `v2/sqlitedrv`
- ~~`pgtool`~~ → `v2/pgdrv`
