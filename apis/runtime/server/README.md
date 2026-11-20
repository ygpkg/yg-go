# apis/runtime/server

yg-go 下游服务共用的 HTTP 装配层：**在构造期声明前缀，通过路由组投递注册，并按三层编排中间件**。

> 版本前提：自 **v1.27.5** 起本包是破坏式 API（注册入口从 `Router` 收敛到路由组、中间件链从硬编码改为可编排的内置条目）。
> 从 1.23 线迁移的服务先看 [从 v1.23 线迁移](#从-v123-线迁移)。

> **命名调整**：`Scope`→`RouterGroup`、`Router.Scope`→`Router.RouterGroup`、`AllScope`→`AllGroups`、
> `PrefixGroup`→`PrefixSpec`、`WithPrefixGroup(s)`→`WithPrefix`/`WithPrefixSpecs`。只改名字与文件归属，
> 查表/广播/启动期 panic 的语义完全不变。

## 目录

- [一分钟上手](#一分钟上手)
- [核心模型（三条规则）](#核心模型三条规则)
- [前缀声明](#前缀声明)
- [路由组注册](#路由组注册)
- [多版本接口并存](#多版本接口并存)
- [注册 API 一览](#注册-api-一览)
- [handler 的四种签名](#handler-的四种签名)
- [中间件：三层与内置条目](#中间件三层与内置条目)
- [账号体系与鉴权策略](#账号体系与鉴权策略)
- [文档路由（swagger / redoc）](#文档路由swagger--redoc)
- [诊断与逃生口](#诊断与逃生口)
- [启动期 panic 清单](#启动期-panic-清单)
- [从 v1.23 线迁移](#从-v123-线迁移)
- [常见坑](#常见坑)

## 一分钟上手

```go
package main

import (
	"net"

	"github.com/gin-gonic/gin"
	"github.com/ygpkg/yg-go/apis/runtime/server"
	"github.com/ygpkg/yg-go/lifecycle"
)

func main() {
	// 1. 构造：前缀在构造期声明，精确集合（不再有构造参数前缀，也没有默认前缀）
	svr := server.NewRouter(server.WithPrefixes(server.PrefixAPIV3, server.PrefixAPIV4))

	// 2. 注册：先把路由组取成变量，再在变量上注册（推荐写法，读起来与 1.23 线一致）
	v3 := svr.RouterGroup(server.PrefixAPIV3)
	v3.P("/user/login", login)
	// 或者：显式广播到全部已声明前缀（必须写出来）
	svr.All().G("/common/ping", ping)

	// 3. 启动：Run 立即返回（内部起 goroutine），由 lifecycle 阻塞并处理退出信号
	l, err := net.Listen("tcp", ":8080")
	if err != nil {
		panic(err)
	}
	if err := svr.Run(l); err != nil {
		panic(err)
	}
	lifecycle.Std().WaitExit() // 等 SIGTERM/SIGINT 或主流程 Exit，然后跑关闭钩子
}

func login(ctx *gin.Context) { ctx.JSON(200, gin.H{"code": 0}) }
func ping(ctx *gin.Context)  { ctx.String(200, "pong") }
```

## 核心模型（三条规则）

1. **前缀只有一个来源**：`NewRouter` 的 option 里声明（`WithPrefixes` / `WithPrefix` / `WithPrefixSpecs`），精确集合。
   `"/v2"`、`"/v2/"`、`"v2"` 会被归一化为同一个前缀 `/v2`，`path.Clean` 折叠内部 `//`（`/v3//x`→`/v3/x`、`//`→`/`），按声明顺序去重、注册；含 `.`/`..` 段的前缀会直接 panic（不静默折叠成根前缀）。未声明的前缀不能注册。
2. **注册必须落在路由组上**：`Router` 自身**没有任何注册方法**。
   `svr.RouterGroup(prefix)` 投单个前缀，`svr.All()` 广播到全部前缀——「广播」只能是显式选择。
3. **装配错误一律启动期 panic**：空前缀集合、未声明前缀、重复注册、非法中间件锚点，都在 `Run` 之前炸掉，
   不会带着半装配的路由表上线，也不会在运行期变成 404。

## 前缀声明

```go
// 只有前缀，没有前缀级中间件（版本前缀直接用 server 包常量）
svr := server.NewRouter(server.WithPrefixes(server.PrefixAPIV3, server.PrefixAPIV4))
```

某个前缀要额外挂中间件时（该前缀下所有路由共享）：

```go
svr := server.NewRouter(
	server.WithPrefixes(server.PrefixAPIV3),
	server.WithPrefix(server.PrefixAPIV4, middleware.AuthMiddleWare),
)
```

批量声明：

```go
svr := server.NewRouter(server.WithPrefixSpecs(
	server.PrefixSpec{Prefix: server.PrefixAPIV3},
	server.PrefixSpec{Prefix: server.PrefixAPIV4, Middlewares: []gin.HandlerFunc{traceMiddleware}},
))
```

**版本前缀常量**：`server.PrefixAPIV1` ~ `server.PrefixAPIV5`，取值是归一化形式（`/v1`…`/v5`，无尾斜杠），
与 `WithPrefixes` / `RouterGroup` / `Routes().Prefix` 同一口径，可直接比较：

```go
for _, route := range svr.Routes() {
	if route.Prefix == server.PrefixAPIV4 { /* v4 的路由 */ }
}
```

- 不再提供 `PrefixAPIDefault`：没有「默认前缀」这回事（决策 D8；D3 管的是「前缀精确集合」）。
- 常量不带尾斜杠，**不能直接与相对路径相加**；拼接时补斜杠：`server.PrefixAPIV3 + "/account"`，
  或 `path.Join(server.PrefixAPIV3, "account")`（注意 `path.Join` 会吞掉相对路径尾斜杠）。
- 多段前缀（如 `/api/v1`）由服务侧定义完整常量，不要用 `"/api" + server.PrefixAPIV1` 拼。
- 前缀级中间件排在**引擎级链之后**、路由级中间件之前。
- 归一化后形如 `/v3`（无尾斜杠）；根路径为 `/`。
- 空字符串前缀会 panic；一个前缀都没声明也会 panic。

## 路由组注册

```go
// 推荐：先把路由组取成变量，再在变量上注册
v2 := svr.RouterGroup("/v2")
v2.P("/user/login", login) // 只投 /v2，其余前缀访问该路径是 404

all := svr.All()
all.P("/common/ping", ping) // 广播到全部已声明前缀，按声明顺序
```

> 链式 `svr.RouterGroup("/v3").P(...)` 也能用，但只适合一次注册的临时场合。正式装配请把路由组 / 广播入口绑定成变量
> （或作为 `Registrar` 传给模块函数）：一处取、多处注册，意图更清楚，也不会重复查前缀。

模块路由函数只依赖 `Registrar` 接口（`RouterGroup` 与 `AllGroups` 都实现），由装配方决定投给谁：

```go
// 模块侧
func Routers(r server.Registrar, deps *Deps) {
	r.P("/user/login", deps.UserLogin)
	r.PRequireLogin("/user/profile", deps.UserProfile)
	r.GWith([]gin.HandlerFunc{rateLimit}, "/user/list", deps.UserList)
}

// 装配侧：同一份模块代码可以选择投递范围
v3 := svr.RouterGroup("/v3")
Routers(v3, deps)        // 只投 v3
Routers(svr.All(), deps) // 广播
```

部分前缀同构（如 V3 有 A、B，V4 有 A、C）不需要广播，逐个路由组注册即可：

```go
// 承接上文 svr
v3, v4 := svr.RouterGroup("/v3"), svr.RouterGroup("/v4")
v3.P("/a", handlerA)
v3.P("/b", handlerB)
v4.P("/a", handlerA)
v4.P("/c", handlerC)
```

## 多版本接口并存

多版本就是**多个已声明前缀**：旧接口留在旧前缀、新接口挂新前缀，互不干扰；需要「所有版本都有」时显式 `All()`。

```go
svr := server.NewRouter(server.WithPrefixes("/v3", "/v4"))
```

### 三种摆放方式

```go
// 承接上文已创建的 svr
v3, v4 = svr.RouterGroup("/v3"), svr.RouterGroup("/v4")
all := svr.All()
```

| 目标 | 写法 | 结果 |
|---|---|---|
| 只在新版本提供 | `v4.P("/user.login", h)` | `/v4/user.login` 有；`/v3/user.login` 是 404 |
| 老版本继续保留 | `v3.P("/legacy.op", h)` | 只有 `/v3/legacy.op` |
| 所有版本都挂 | `all.P("/common.ping", h)` | 每个声明前缀各一条（显式广播） |

### 同一 path 在两个版本用不同实现（灰度 / 重写）

```go
v3, v4 = svr.RouterGroup("/v3"), svr.RouterGroup("/v4")
v3.P("/user.login", legacyLogin) // 老实现
v4.P("/user.login", newLogin)    // 新实现
```

重复检测的键是「method + 完整路径（含前缀）」，所以这两条注册不冲突——`Routes()` 会分别列出
`POST /v3/user.login`、`POST /v4/user.login`。这是多版本并存最常用的形态：先让新版本跑新实现，老客户端继续走老版本，确认后再下线。

### 部分重叠（V3 有 A、B，V4 有 A、C）

```go
// 承接上文 v3、v4
v3.P("/a", handlerA)
v3.P("/b", handlerB)
v4.P("/a", handlerA)
v4.P("/c", handlerC)
```

不要为了让两版都拿到 A 而用 `All()`——那会把 B、C 也铺到另一版。

### 版本级差异

- 前缀级中间件：`server.WithPrefix("/v4", mws...)`
- 版本级账号体系/鉴权：`server.WithPrefixAuthPolicy("/v4", policy)`（见下一节）
- 文档：`v4.Docs("app")` 与 `v3.Docs("app")` 各自挂载；只想对外承诺 v4，就只给 v4 挂
- 前缀级路由写在同一处时，用局部变量区分：`v3, v4 := svr.RouterGroup("/v3"), svr.RouterGroup("/v4")`

### 版本下线

- 迁移期：新旧前缀并存，按接口逐个把注册从旧路由组挪到新路由组（或复制一条新实现）；
- 确认旧版本无流量后，**删除该路由组上的注册**即可，路径自然变成 404；
- 不要用「删除 `WithPrefixes` 里某个前缀」来下线：该前缀一旦未声明，所有 `RouterGroup(旧前缀)` 都会在启动期 panic，部署直接失败。

### 对账与排查

启动日志会打印前缀清单与链长度：

```
INFO router assembled: prefixes=[/v3 /v4] chain_len=8 builtin=[...]
```

按前缀统计已注册路由：

```go
counts := map[string]int{}
for _, route := range svr.Routes() {
	counts[route.Prefix]++
}
log.Printf("routes per prefix: %v", counts)
```

迁移前后各导一次 `Routes()` 的 `method,path` 快照做 diff，就能发现「少了一个版本」这类问题。

### 注意

- 访问未声明的版本（如 `/v5/...`）走默认 `NoRoute`：404 + `The incorrect API route.`，不是 500。
- 前缀归一化：`/v3` 与 `/v3/` 是同一个前缀；`Routes().Prefix` 返回归一化后的 `/v3`。
- 不要用「一个 handler 里判断版本号」来代替两个路由组的分别注册：版本分支一旦进业务代码，路由清单与文档就再也对不上。

## 注册 API 一览

| 方法（`RouterGroup` / `AllGroups` / `Registrar`） | 说明 |
|---|---|
| `P(action, hdrs...)` | 注册 POST 路由 |
| `G(action, hdrs...)` | 注册 GET 路由 |
| `Any(action, hdrs...)` | 注册全部 HTTP 方法（`Routes()` 里记为一条 `ANY`） |
| `PWith(mws, action, hdrs...)` | POST + 路由级中间件（`[]gin.HandlerFunc`，排在 handler 之前） |
| `GWith(mws, action, hdrs...)` | GET + 路由级中间件 |
| `PRequireLogin(action, hdrs...)` | 纯糖：`PWith([]gin.HandlerFunc{AuthPolicy.Login()}, ...)`，默认即 `middleware.AuthMiddleWare` |
| `PRequireEmployee(action, hdrs...)` | 纯糖：`PWith([]gin.HandlerFunc{AuthPolicy.Employee()}, ...)`，默认即 `middleware.AuthMiddleWareEmployee` |
| `GRequireLogin(action, hdrs...)` | 纯糖：`GWith([]gin.HandlerFunc{AuthPolicy.Login()}, ...)` |
| `Docs(model)` | 把 swagger / redoc 挂到该路由组，见[文档路由](#文档路由swagger--redoc) |

`hdrs ...interface{}` 的语义与 1.23 线一致：可以混排 handler 与 `gin.HandlerFunc`，按传入顺序成为 handler 链。
把中间件塞进 `hdrs`（`rg.P("/x", gated, handler)`）与显式用 `PWith` 等价。

## handler 的四种签名

| 写法 | 适配行为 |
|---|---|
| `func(ctx *gin.Context, in *TIn, out *TOut) error` | 反射适配：请求体 JSON 解到 `in`，调用后把 `out` 序列化为响应；`out` 内嵌 `apiobj.BaseResponse` 时自动补 `message` / `request_id` / `env` 并回填 `code`；返回的 error 走统一错误响应 |
| `gin.HandlerFunc` / `func(*gin.Context)` | 直接作为 gin handler |
| `http.HandlerFunc` / `func(http.ResponseWriter, *http.Request)` | 包装成 gin handler |
| `http.Handler` | 包装成 gin handler |

```go
type UserLoginReq struct {
	Account string `json:"account"`
}

type UserLoginResp struct {
	apiobj.BaseResponse
	Token string `json:"token,omitempty"`
}

func UserLogin(ctx *gin.Context, in *UserLoginReq, out *UserLoginResp) error {
	out.Token = "token-for-" + in.Account
	return nil
}

v3 := svr.RouterGroup("/v3")
v3.P("/user/login", UserLogin)
```

## 中间件：三层与内置条目

### 引擎级：8 个内置条目

链顺序由库保证。定制时直接引用**条目定义**（`server.Recovery`、`server.Logger` …，不是名字字符串），**不需要也不能**自己排列整条链：

| 顺序 | 条目定义 | 作用 | 关掉之后的直接后果 |
|---|---|---|---|
| 1 | `server.Recovery` | panic 兜底，返回统一错误 | 未捕获 panic 直冲 net/http，客户端见连接重置 |
| 2 | `server.CORS` | 跨域响应头 | 浏览器端跨域请求全部失败 |
| 3 | `server.CustomerHeader` | 注入 reqid、回写 `Env` 响应头 | `runtime.RequestID(ctx)` 的 MustGet panic，`BaseResponse.RequestID` 为空 |
| 4 | `server.Logger` | 访问日志与请求直方图 | 无访问日志、无 `request_latency_seconds` |
| 5 | `server.LoginStatus` | 解析 token 写入登录态 | 业务代码读 `runtime.LoginStatus` 会 MustGet panic（框架 `Inject` 已跳过）；默认 `AuthPolicy` 在无登录态时 fail-closed 返回 401（不 panic），自研账号体系需配合 `WithAuthPolicy` |
| 6 | `server.Language` | 写入 `Accept-Language` | i18n 退化为默认语言 |
| 7 | `server.Inject` | 按 issuer 调用鉴权注入器 | 注入器不执行，`LoginStatus.Err` 恒为空 |
| 8 | `server.IdentityFields` | 把 companyid/uin 写进请求日志上下文 | 访问日志退回从 ctx 读 uin，companyid 缺失 |

三条不变量由代码 + 单测保证：

- `server.Recovery` 固定链首（**不能**在它之前插入）；
- `server.IdentityFields` 固定末位，且晚于 `server.Inject`（companyid/uin 由注入器在 Inject 里写入）；
- `WithMiddleware` 的尾插永远晚于 `server.IdentityFields`。

### 编排 API

关闭某个内置条目（直接传条目定义，不是名字字符串；启动日志会列出被禁项 + 受影响 helper）：

```go
svrNoCORS := server.NewRouter(
	server.WithPrefixes(server.PrefixAPIV3),
	server.WithoutMiddleware(server.CORS),
)
```

替换实现 + 定制条目参数（同一条目不能既 `WithoutMiddleware` 又 `WithMiddlewareImpl`，同时出现会在构造期 panic）：

```go
svr := server.NewRouter(
	server.WithPrefixes("/v3"),

	// 在某个内置条目之前新增中间件（锚点不能是 server.Recovery）
	server.WithMiddlewareBefore(server.Logger, traceMiddleware),

	// 替换内置实现（不用于新增）；中间件自身的配置走 middleware 包的选项
	server.WithMiddlewareImpl(server.Recovery, myRecovery),
	// Logger 白名单：默认 {".Ping", "metrics"}，这里整体覆盖
	server.WithMiddlewareImpl(server.Logger,
		middleware.Logger(middleware.WithLoggerWhitelist(".Ping", "metrics", "/healthz"))),
	// CORS：默认与 1.23 线一致，按场景做定向扩展（来源 / 自定义头 / 暴露头 / 预检缓存…）
	server.WithMiddlewareImpl(server.CORS,
		middleware.CORS(
			middleware.WithCORSAllowOrigins("https://app.example.com", "https://admin.example.com"),
			middleware.WithCORSAllowHeaders("X-Tenant-Id"),
			middleware.WithCORSExposeHeaders("X-Request-Id"),
			middleware.WithCORSMaxAge(2*time.Hour),
		)),

	// 追加到链尾（晚于 server.IdentityFields，适合 RequestLogFields 这类「在身份字段之后追加」的中间件）
	server.WithMiddleware(middleware.RequestLogFields(func(ctx *gin.Context) []interface{} {
		return []interface{}{"tenanttype", ctx.GetString("tenanttype")}
	})),
)
```

非法组合在构造期 panic：锚点不存在、锚点被 `WithoutMiddleware` 关掉、锚点是 `server.Recovery`、传了零值条目、或同一条目既禁用又替换。

### 各中间件的选项（按中间件归属命名）

选项名带中间件前缀，避免 `WithWhitelist` 这种「不知道属于谁」的歧义：Logger 的白名单是
`WithLoggerWhitelist`。鉴权侧没有中间件级白名单（见下面「鉴权侧的接口白名单」），
所以不会出现归属不明的白名单选项。

| 中间件 | 选项 | 对应实际场景 |
|---|---|---|
| `server.Logger` | `middleware.WithLoggerWhitelist(items ...string)` | 探针/静态路径不进访问日志（server 内置条目默认 `{".Ping", "metrics"}`；替换实现时用 `server.DefaultLoggerWhitelist()` 保留默认项再加自己的路径） |
| `server.CORS` | `middleware.WithCORSAllowOrigins(origins ...string)` | 指定前端域名，替代默认的「允许所有来源」 |
| | `middleware.WithCORSAllowOriginFunc(verify func(string) bool)` | 多环境/租户子域/preview 域名的动态校验 |
| | `middleware.WithCORSAllowHeaders(headers ...string)` | 新增自定义请求头（`X-Tenant-Id`、`X-Client-Version`…），默认头保留 |
| | `middleware.WithCORSExposeHeaders(headers ...string)` | 暴露响应头给前端（`X-Request-Id`、分页头…） |
| | `middleware.WithCORSAllowMethods(methods ...string)` | 调整允许的方法集（默认 GET/POST/PUT/DELETE/PATCH） |
| | `middleware.WithCORSAllowCredentials(allow bool)` | 是否允许携带凭证（默认允许） |
| | `middleware.WithCORSMaxAge(age time.Duration)` | 预检结果缓存时间（默认 12h） |
| 其余 6 个条目 | 暂无可选项 | 需要完整控制时用 `WithMiddlewareImpl(server.X, 自定义中间件)` 整体替换，例如 `WithMiddlewareImpl(server.CORS, cors.New(fullConfig))`；替换 `server.Logger` 时记得用 `server.DefaultLoggerWhitelist()` 保留默认白名单 |

> CORS 的 `AllowWildcard` 没有暴露：`WithCORSAllowOrigins("https://*.example.com")` 这类通配来源会通过校验但实际不匹配（全部 403）；通配/动态来源请用 `WithCORSAllowOriginFunc`，或整条替换 CORS 实现。


所有 option 只能在 `NewRouter` 调用中应用，构造完成后再调用任一 option 会 panic（`sealed`）。

装配完成会打印一行启动日志，便于对账：

```
INFO router assembled: prefixes=[/v3 /v4] chain_len=7 builtin=[recovery customer_header logger login_status language inject identity_fields]
WARN middleware "cors" disabled: 浏览器端跨域请求全部失败
```

### 前缀级 / 路由级

```go
// 前缀级：只作用于该前缀下的全部路由
svr := server.NewRouter(server.WithPrefix("/v4", middleware.AuthMiddleWare))

// 路由级：只作用于这一条路由，中间件排在 handler 之前
v4 := svr.RouterGroup("/v4")
v4.PWith([]gin.HandlerFunc{rateLimit, audit}, "/order/create", createOrder)

// 1.23 风格的鉴权糖（与 PWith 等价的薄糖，可混用）
v4.PRequireLogin("/user/profile", userProfile)
```

## 账号体系与鉴权策略

`PRequireLogin` / `PRequireEmployee` / `GRequireLogin` 是「登录态 / 员工身份」的注册糖，背后的准入中间件
由 `AuthPolicy` 提供，可以整体替换：yg-go JWT、自研会话、外部身份网关都能沿用同一套注册语法。

### 默认策略

`DefaultAuthPolicy()` 就是 1.23 线的行为：

| 糖 | 默认 guard | 判定 |
|---|---|---|
| `PRequireLogin` / `GRequireLogin` | `middleware.AuthMiddleWare` | `runtime.LoginStatus(ctx).State != StateSucc` → 200 + `{"code":401,"message":"unauthorized"}` |
| `PRequireEmployee` | `middleware.AuthMiddleWareEmployee` | 额外要求 `Role == RoleEmployee` |

### 使用 yg-go 内置鉴权体系（corekg / dotpen 形态）

大多数服务用的是 yg-go 自带的那套：`server.LoginStatus` 解析 token → `svr.Inject` 调注入器补身份 →
`PRequireLogin` / `PRequireEmployee` 判准入。这条链路全开时零配置，只需要注册注入器。

**请求期顺序**

| 顺序 | 阶段 | 做什么 |
|---|---|---|
| 1 | `server.LoginStatus`（内置） | 读 `Authorization: Bearer <token>`：`yg-` 前缀走 API Key 分支（`Role=RoleAPI`、`State=StateSucc`，不验签）；其余按 JWT 解析，密钥由 `auth.GetJwtSecret(ctx, issuer)` 从 settings 的 JWT 配置取，取不到时用 `auth.SetJwtSecretResolver` 注册的兜底解析器。解析失败 → `State=StateFailed` |
| 2 | `server.Inject`（内置，`svr.Inject`） | 仅当 `State=StateSucc` 时执行注入器：把用户/公司/API Key 等 ID 通过 `ls.SetID(...)` 写进 idmap，判定账号/成员状态，必要时改写 `Role`；返回 error 或把 `State` 置成 `StateFailed` 都会让后面的准入拒绝 |
| 3 | `server.IdentityFields`（内置） | 把 `companyid` / `uin` 写进请求日志上下文（访问日志与业务日志共用） |
| 4 | 路由级 guard | `PRequireLogin` 判 `State == StateSucc`；`PRequireEmployee` 额外要求 `Role == RoleEmployee`。默认返回 200 + `{"code":401,"message":"unauthorized"}` |
| 5 | handler | 业务代码用 `runtime.LoginStatus(ctx)` / `runtime.Uin(ctx)` / `runtime.CompanyID(ctx)` 取身份 |

> 表里只列与身份/鉴权相关的阶段；`server.Logger`、`server.CORS` 等包装层按完整链顺序排在前后的其他位置，不影响这个顺序（完整链见上一节）。

**装配示例（与 corekg 一致）**

```go
svr := server.NewRouter(server.WithPrefixes("/v3"))

// 注入器按 token 的 iss（issuer）路由：不同签发方可以填不同的身份信息。
// "" 等价于 Default(...)：没匹配到 issuer 时兜底。
svr.AuthInject("yygu", accountmds.InjectLoginStatus)
svr.AuthInject("dotpen", accountmds.InjectLoginStatus)
svr.Default(accountmds.InjectLoginStatus)

v3 := svr.RouterGroup("/v3")
v3.PRequireLogin("forest.ListForest", forestctl.ListForest)
v3.PRequireLogin("forest.GetForest", mds.HasForestUsePerm, forestctl.GetForest)
v3.PRequireEmployee("admin.ListUsers", adminctl.ListUsers)
```

**注入器契约**（corekg 的 `accountmds.InjectLoginStatus` 是同一形状）：

```go
func InjectLoginStatus(ctx *gin.Context, ls *auth.LoginStatus) error {
	if ls.Role == auth.RoleAPI {
		// API Key 分支：自行校验 key 并把 keyID 写进 idmap
		ls.SetID(global.CtxKeyAPIKeyID, keyID)
		return nil
	}
	if ls.State != auth.StateSucc {
		return nil // 未登录/解析失败：交给 PRequireLogin 判，不在这里写响应
	}
	if ls.Claim.Audience == global.AudienceUser {
		ls.Role = auth.RoleUser
	}
	ls.SetID(global.CtxKeyUin, ls.Claim.Uin)         // 日志 uin
	ls.SetID(global.CtxKeyCompanyID, companyID)      // 日志 companyid
	if accountDisabled {
		// 账号/成员状态异常：置 Failed + 返回 error，RequireLogin 会拒绝
		ls.State = auth.StateFailed
		return errAccountDisabled
	}
	return nil
}
```

**Role 语义**

| Role | 含义 | `PRequireLogin` | `PRequireEmployee` |
|---|---|---|---|
| `RoleNil` | 未识别（无 token/解析失败） | 拒绝 | 拒绝 |
| `RoleUser` | 普通用户（如 `aud == user`） | 放行 | 拒绝 |
| `RoleEmployee` | 运营/员工 | 放行 | 放行 |
| `RoleAPI` | API Key 调用 | 放行（`State=StateSucc`） | 拒绝 |

**注意**

- 注入器**不要写 HTTP 响应**：它的契约只有「返回 error → `State=Failed`」，非 nil error 最终都会收敛成默认 401 信封。
  需要专用错误码/文案、或需要按 action 豁免的判据（租户、成员资格、幂等）要留在**路由级中间件**，见下一节。
- `PRequireLogin` 之前已经有 `server.LoginStatus`+`server.Inject`，能走到 guard 的请求登录态一定合法；
  业务判据不要再重复判 `State`。
- 只关 `server.LoginStatus` 不影响编译；默认 guard 会 fail-closed 返回 401（不再 500），业务代码里直接调 `runtime.LoginStatus` 仍会 MustGet panic（框架的 `Inject` 已兼容跳过）。
- 想要「匿名可访问、但带了 token 就补身份」的端点：用普通 `P`/`G`（不挂 guard），`server.LoginStatus` 会解析但不拦截。

### 自研账号体系（不经 yg-go LoginStatus）

```go
svr := server.NewRouter(
	server.WithPrefixes("/api/v1"),
	// 不再解析 yg-go token；注入器会自动跳过，不会 MustGet panic
	server.WithoutMiddleware(server.LoginStatus),
	server.WithAuthPolicy(server.NewAuthPolicy(myGate, myAdminGate)),
)

api := svr.RouterGroup("/api/v1")
api.PRequireLogin("file.Upload", upload)
api.PRequireEmployee("admin.Export", export)
```

- guard 是普通 `gin.HandlerFunc`：**自己判、自己写响应、自己 `Abort`**，放行时 `ctx.Next()`。
  框架不包统一 401，所以「身份服务不可达」和「成员资格被撤销」可以各自用业务码（如 20011 / 20012），
  不必都退化成 401。
- 身份解析建议放在 engine 级中间件（`WithMiddleware` / `WithMiddlewareBefore`），guard 只做准入；
  也可以直接在 guard 里解析后写进 `ctx`。
- `NewAuthPolicy(login, employee)` 某个级别传 `nil` 时回退到内置实现，适合同一服务只有一半需要替换。

### 按前缀用不同账号体系（多版本）

```go
svr := server.NewRouter(
	server.WithPrefixes("/v3", "/v4"),
	server.WithAuthPolicy(legacyPolicy),                    // 全局：老账号体系
	server.WithPrefixAuthPolicy("/v4", sessionPolicy),      // /v4 换成新身份网关
)
v3, v4 := svr.RouterGroup("/v3"), svr.RouterGroup("/v4")
v3.PRequireLogin("/user.profile", legacyHandler)
v4.PRequireLogin("/user.profile", newHandler)
```

未覆盖的前缀回退到全局策略；`WithPrefixAuthPolicy` 传了未声明前缀会在构造期 panic。
同一 path 在两个版本分别用两套账号体系，是这套机制支持的显式形态。

### 新版本鉴权中间件（完整示例）

典型场景：**v3 继续用 yg-go 登录态，v4 改用新身份网关（BFF 会话）**。要点是「新版本的身份解析挂前缀级、
准入 guard 通过 `AuthPolicy` 换掉」，两者都只作用于 `/v4`，v3 完全不受影响。

```go
// 1) 新版本的身份解析：前缀级中间件，只挂在 /v4 上。
//    只解析、不写响应；失败也把错误放进 ctx，响应形态交给 guard —— 保证 401 / 20011 / 20012 只有一处决定。
func v4SessionResolver(verify func(*gin.Context) (*Session, error)) gin.HandlerFunc {
    return func(ctx *gin.Context) {
        sess, err := verify(ctx)
        if err != nil {
            ctx.Set(ctxKeyV4SessionErr, err)
            ctx.Next()
            return
        }
        ctx.Set(ctxKeyV4Session, sess)
        ctx.Next()
    }
}

// 2) 新版本的准入 guard：实现 AuthPolicy。
type v4Policy struct{}

func (v4Policy) Login() gin.HandlerFunc {
    return func(ctx *gin.Context) {
        if _, ok := ctx.Get(ctxKeyV4Session); !ok {
            // 自己的错误信封：未登录与「身份服务不可达」分开表达
            ctx.AbortWithStatusJSON(http.StatusOK, gin.H{"code": 401, "message": "session required"})
            return
        }
        ctx.Next()
    }
}

func (v4Policy) Employee() gin.HandlerFunc {
    return func(ctx *gin.Context) { /* 新网关里的成员 / 角色判定，同样自写响应 */ }
}

// 3) 装配：全局策略保持默认（v3 用），只给 /v4 换解析中间件与 guard。
svr := server.NewRouter(
    server.WithPrefixes("/v3", "/v4"),
    server.WithPrefix("/v4", v4SessionResolver(verifier)), // 只对 /v4 生效
    server.WithPrefixAuthPolicy("/v4", v4Policy{}),             // 只影响 /v4 的三个糖
)

v3, v4 := svr.RouterGroup("/v3"), svr.RouterGroup("/v4")
v3.PRequireLogin("/user.profile", legacyProfile) // 默认策略：读 yg-go LoginStatus
v4.PRequireLogin("/user.profile", newProfile)    // v4Policy：读新网关会话
```

执行顺序与边界：

| 顺序 | 层 | v3 请求 | v4 请求 |
|---|---|---|---|
| 1 | 引擎级（8 项链） | `server.LoginStatus` 解析 token | 同样执行（没带 Authorization 就是匿名，不报错） |
| 2 | 前缀级 | 无 | `v4SessionResolver` 解析 BFF 会话 |
| 3 | 路由级 guard | 默认 `AuthPolicy`（`AuthMiddleWare`） | `v4Policy`（`WithPrefixAuthPolicy` 生效） |
| 4 | handler | `legacyProfile` | `newProfile` |

- **不要因为 v4 不用 yg-go 登录态就全局 `WithoutMiddleware(server.LoginStatus)`**：v3 还依赖它。
  只有整个服务都不使用 yg-go 登录态时（如 mutistitch 形态）才全局关闭，此时 `Inject` 自动跳过。
- 前缀级中间件在构造期挂一次，对该前缀**所有路由**生效；guard 是**路由级**的，按注册时解析到的策略拼进 handler 链。
- v4 的 handler 不要调用 `runtime.LoginStatus`，改为从自己的 ctx 取会话；要记录自有身份维度，
  用 `WithMiddleware(middleware.RequestLogFields(...))` 尾插。
- 同一份模块函数可以被两个版本共用（签名收 `Registrar`），版本差异只体现在装配：
  `Routers(svr.RouterGroup("/v3"), deps)` 与 `Routers(svr.RouterGroup("/v4"), deps)`。
- 验证方式：`httptest` 分别打 `/v3/user.profile` 与 `/v4/user.profile`，断言两套拒绝形态与放行路径；
  `Routes()` 里同一 path 出现两条、`Prefix` 不同。

### 鉴权侧的「接口白名单」

鉴权没有 `WithAuthWhitelist` 这类中间件级白名单，因为**路由级挂载本身就是白名单**：
不需要登录的接口用普通 `P`/`G` 注册（不挂 `PRequireLogin`），受保护接口才挂 guard。

如果确实需要「统一挂一个 guard、再按接口豁免」（例如把 guard 挂在前缀级），用自定义 `AuthPolicy`
在 guard 内部判断，而不是给 guard 加一个路径列表：

```go
type whitelistPolicy struct {
    skip map[string]bool // 例：map[string]bool{"POST /v3/auth.LoginURL": true}
}

func (p whitelistPolicy) Login() gin.HandlerFunc    { return p.guard(false) }
func (p whitelistPolicy) Employee() gin.HandlerFunc { return p.guard(true) }

func (p whitelistPolicy) guard(requireEmployee bool) gin.HandlerFunc {
    return func(ctx *gin.Context) {
        if p.skip[ctx.Request.Method+" "+ctx.FullPath()] {
            ctx.Next()
            return
        }
        // ... 正常登录态判定；requireEmployee 时再校验角色 + 自己的拒绝响应
    }
}
```

### 路由级追加判据

账号体系之外、需要**按 action 豁免**的准入（租户校验、幂等、限流、请求体守卫等）放在路由级，
`PRequireLogin`/`PWith` 的 `hdrs` 会按顺序排在 guard 之后：

```go
rg.PRequireLogin("color.CreatePalette", requireIdentity, idempotency, writeLimit, createPalette)
rg.PWith([]gin.HandlerFunc{requireIdentity}, "user.Profile", profile)
```

这也是 canvas2 的既有形态：登录态由框架 guard 判，业务判据留路由级，才能保证「凭证未绑定空间」
这类状态下的个别端点（如切换空间）可以单独豁免。

### 与「身份解析」的分工

| 层 | 归属 | 扩展点 |
|---|---|---|
| 解析 token / 会话并填充登录态 | 服务或 yg-go 内置 | `server.LoginStatus` + `AuthInject`/`Default` 注入器；自研体系关掉 `server.LoginStatus` |
| 判定是否放行 | `AuthPolicy` | `WithAuthPolicy` / `WithPrefixAuthPolicy` / `NewAuthPolicy` |
| 单条路由的额外判据 | 路由级中间件 | `PWith` / `GWith` / `hdrs` |

自研体系关闭 `server.LoginStatus` 后，`svr.Inject` 会直接跳过（不再 `MustGet` panic），
`server.IdentityFields` 会把 companyid/uin 记为 0；要记录自己的身份维度，用 `WithMiddleware(middleware.RequestLogFields(...))` 追加。

## 文档路由（swagger / redoc）

文档**按路由组挂载**，不再自动铺满全部前缀：

```go
svr.All().Docs("app") // 每个已声明前缀各挂两条：/{prefix}/app.docs/*any、/{prefix}/app.redocs

v4 := svr.RouterGroup("/v4")
v4.Docs("app") // 只挂 /v4，其他前缀访问文档是 404
```

没挂 `Docs` 的前缀就是没有文档路由（默认 404）——这是刻意的：文档对外承诺哪些版本，由挂载决定。

## 诊断与逃生口

### 路由清单

```go
for _, route := range svr.Routes() {
	log.Printf("%s %s prefix=%s source=%s", route.Method, route.Path, route.Prefix, route.Source)
}
svr.LogRoutes() // 同上，直接打印
```

`Routes()` 返回注册顺序的快照副本，四元组：`Method` / `Path`（含前缀的完整路径，与 gin 实际注册一致）/
`Prefix` / `Source`（注册发生位置 `file:line`）。`Any` 注册的条目 `Method` 记为 `ANY`（对应 gin 的 9 个方法，
重复检测按 9 个方法键分别登记）。重复注册的 panic 消息会同时给出**新旧两个来源**：

```
server: duplicate route "POST /v3/user/login", already registered by /path/router.go:42, re-registering from /path/router.go:57
```

### `GinEngine()`：逃生口

探针、`NoRoute`、静态文件这类「非业务动作」直接挂在 engine 上，不进 `Routes()` 索引，也不参与重复检测：

```go
svr.GinEngine().GET("/healthz", healthz)
svr.GinEngine().Static("/assets", "./assets")
svr.GinEngine().NoRoute(func(ctx *gin.Context) { ctx.String(404, "not found") }) // 会覆盖内置的 404 文本
```

但**不要**用它挂业务路由：那样不会进注册清单，也拿不到重复检测与 `Routes()` 对账。

## 启动期 panic 清单

| 场景 | panic 消息关键字 |
|---|---|
| 一个前缀都没声明（含 `WithPrefixes()` 空参数） | `no prefix declared` |
| 前缀是空字符串 | `prefix must not be blank` |
| `RouterGroup` 用了未声明的前缀 | `undeclared prefix` + `available prefixes` |
| 同 method + 同 path 在同一前缀二次注册 | `duplicate route` + 两个 `file:line` |
| `WithMiddlewareBefore` 锚点不存在 / 是 `server.Recovery` / 已被关闭 | `unknown middleware` / `must stay first` / `disabled by WithoutMiddleware` |
| 同一条目既 `WithoutMiddleware` 又 `WithMiddlewareImpl` | `disabled by WithoutMiddleware` + `WithMiddlewareImpl` |
| 任何中间件 option 传零值/未知条目 | `unknown middleware` |
| `WithMiddlewareImpl(entry, nil)` | `must not be nil` |
| `WithAuthPolicy(nil)` / `WithPrefixAuthPolicy(..., nil)` | `policy must not be nil` |
| `WithPrefixAuthPolicy` 空前缀 / 未声明前缀 | `prefix must not be blank` / `undeclared prefix` + `WithPrefixAuthPolicy` |
| `WithPrefixes` 里有空字符串 | `prefix must not be blank` |
| `Any` 与 `P/G` 注册同一 path | `duplicate route` + 两个 `file:line` |
| `AuthPolicy` 返回 nil guard | `returned nil` + `AuthPolicy` |
| `PWith` 的路由中间件里有 nil / handler 为 nil | `route middleware` / `handler must not be nil` |
| `NewRouter` 传 nil option | `RouterOption must not be nil` |
| `Run(nil)` / 重复 `Run` | `listener must not be nil` / `already running` |
| `Run` 之后再注册路由 | `registration must happen before Run` |
| 前缀含 `.`/`..` 段 | `must not contain` + `segment` |
| `NewRouter` 之后再应用 option | `sealed` |

## 从 v1.23 线迁移

| 1.23 线写法 | 1.27 线写法 |
|---|---|
| `NewRouter(apiPrefix, opts...)` | `NewRouter(WithPrefixes("/v3", ...), opts...)` |
| `WithPrefixes([]string{V3, V4})`（与构造参数取并集） | `WithPrefixes(V3, V4)`（精确集合） |
| `svr.P/G/Any(...)`（隐式广播） | `v4 := svr.RouterGroup(V4); v4.P(...)` 或 `svr.All().P(...)` |
| `svr.PRequireLogin(...)` | `rg.PRequireLogin(...)`（同名糖，底层 guard 可用 `AuthPolicy` 替换）或 `rg.PWith(...)` |
| `svr.PRequireEmployee(...)` | `rg.PRequireEmployee(...)` 或 `rg.PWith([...]AuthMiddleWareEmployee, ...)` |
| `svr.GRequireLogin(...)` | `rg.GRequireLogin(...)` 或 `rg.GWith([...]AuthMiddleWare, ...)` |
| `svr.HandleDoc("app")` | `svr.All().Docs("app")` 或 `v3 := svr.RouterGroup(V3); v3.Docs("app")` |
| `svr.ListAllRouters()` | `svr.Routes()` / `svr.LogRoutes()` |
| `WithDeployMode(mode)` | 删除（原本只写不读）；部署差异用中间件或各自 option 表达 |
| `server.PrefixAPIV3` / `PrefixAPIDefault` | 版本常量保留为 `server.PrefixAPIV1` ~ `PrefixAPIV5`（归一化、无尾斜杠）；注意 `PrefixAPIV3` 的值由 `"/v3/"` 变为 `"/v3"`，旧代码 `PrefixAPIV3 + "account"` 会拼成 `/v3account`，要补斜杠；`PrefixAPIDefault` 删除（不再有默认前缀语义） |
| `server.MethodFunc` | 无对应公开符号（handler 适配收进内部 `register/adaptHandlers`）；直接传 handler 即可 |
| `svr.Prefix`（读字段） | 6 处读取点所在模块都是单前缀 app，直接用 `server.PrefixAPIVx` / 服务自己的常量；但这些模块所属的 app 里有**多前缀 app**（dotpen/dottask、corekg/corekg），注册多版本时必须显式 `All()`，不能靠读单一前缀推导广播 |
| `func Routers(svr *server.Router)` | 纯注册的模块函数改成 `func Routers(r server.Registrar)`（调用点传路由组或 `svr.All()`）；仍需要 `AuthInject` / `GinEngine()`（含 `eng.ContextWithFallback` 这类引擎级设置）的模块保留 `*server.Router`，内部 `eng := svr.RouterGroup(prefix)` 后再注册；`HandleDoc` 已由 `Registrar.Docs` 取代，不需要为它保留 Router |
| `middleware.Logger(".Ping", "metrics")`（直接调用） | 改选项签名：`middleware.Logger(middleware.WithLoggerWhitelist(".Ping", "metrics"))`（白名单配置统一走 middleware 选项；1.23 的 Router 也没有该 option） |
| `middleware.CORS()`（直接调用） | 默认行为不变；按场景用 `middleware.WithCORSAllowOrigins/AllowHeaders/ExposeHeaders/...`，需完整控制时 `server.WithMiddlewareImpl(server.CORS, cors.New(cfg))` |
| `eng.GinEngine().GET(...)` 探针 / `NoRoute` | 不变（逃生口） |
| `hdrs` 里塞 `gin.HandlerFunc` | 不变，或用显式 `PWith/GWith` |

三个典型 diff：

```go
// 声明多个前缀、但路由只投 /v4（1.23 线靠构造参数 PrefixAPIDefault 自动并入 /v4）
-svr := server.NewRouter(server.PrefixAPIDefault, server.WithPrefixes([]string{"/v3/", "/v4/"}))
-Routers(svr)
+svr := server.NewRouter(server.WithPrefixes(server.PrefixAPIV3, server.PrefixAPIV4))
+Routers(svr.RouterGroup(server.PrefixAPIV4))

// 多前缀广播
-svr.P("/user/login", login)
+svr.All().P("/user/login", login)

// 文档
-svr.HandleDoc("app")
+svr.All().Docs("app")
```

**下游迁移检查清单**（按 2026-11 对 canvas2 / mutistitch / dotpen / corekg 的实测整理）：

1. **工具链**：yg-go v1.27.x 的 `go.mod` 是 `go 1.27.4`，消费方 `go 1.23/1.24` 必须同批升级（`go mod tidy` / `go build -mod=mod` 会把消费方 `go` 指令抬到 1.27.4；普通 `go build` 会提示需要更新 `go.mod`）；Dockerfile/CI 镜像同步。
2. **vendor**：有 vendor 的仓要 `go mod vendor` 重新生成（server 包文件数与 `modules.txt` 会变），建议单独一个提交。
3. **`NewRouter`**：全部改成 `NewRouter(WithPrefixes(prefix, ...), opts...)`；测试里的构造点同样要改。
4. **`HandleDoc`**：改成 `svr.All().Docs(model)`（单前缀 app）或 `rg.Docs(model)`。
5. **`Router.Prefix` 读取**（四家共 6 处，读取点所在模块均为单前缀）：换成服务自己的前缀常量；其中 dotpen/dottask、corekg/corekg 是**多前缀 app**，注册必须显式 `All()`，不能靠读单一前缀推导广播。
6. **`WithDeployMode`**：纯删除（旧实现只写不读）。
7. **对账**：迁移前后都用 `GinEngine().Routes()` 导出 `method,path` 集合做 diff（1.23 线没有 `Routes()`；且新 `Routes()` 不含 `GinEngine()` 直挂的探针/静态/NoRoute/引擎子组，单用它会把这类路由误报成丢失）。`Routes()` 只用于业务路由对账。
8. **盘点**：ripgrep 要带 `--no-ignore`（CoreKG-oss 的 `.gitignore` 含 `cmd/`，否则会漏掉生产入口）。
9. **注册 receiver 命名**：canvas2 的两处 layerguard 按 receiver 名 `eng` 做 AST 扫描；推荐「参数改名 `eng`→`svr`、`eng := svr.RouterGroup(prefix)`」的食谱，注册行与守卫都不用改。
10. **已知观测变化**：`Recovery` 提前到链首后，panic 请求不再进访问日志/`request_latency_seconds`（见「常见坑」）。

**迁移必查**：1.23 线里靠「构造参数自动并入」拿到的前缀，必须在 `WithPrefixes` 里显式写出——这类写法迁移后会编译通过但少一个前缀。
保险做法是迁移前后都用 `GinEngine().Routes()` 导出 `method,path` 集合做 diff（`Routes()` 不含引擎直挂路由，不能单独用于对拍）。

## 常见坑

1. **忘记声明前缀** → 构造期 panic（`no prefix declared`）。没有默认前缀。
2. **`RouterGroup` 拼错前缀** → 构造/注册期 panic，消息里带可用前缀清单；不会懒创建。
3. **同一前缀重复注册同一 method+path** → panic 并给出两个注册来源。不同前缀之间同 path 不冲突（这正是路由组模型）。
4. **多前缀服务习惯性写死一个前缀** → 想全量就 `All()`，想部分重叠就逐个 `RouterGroup` 注册，不要再用 1.23 的「构造参数并集」思路。
5. **`WithMiddleware` 不是「想插哪就插哪」**：它是链尾（晚于 `IdentityFields`）；要插在某内置项之前用 `WithMiddlewareBefore`。
6. **中间件必须在构造期装**：`NewRouter` 之后不能再应用 option（panic `sealed`），这是为了避免「后挂中间件对已有路由不生效」的时序陷阱。
7. **探针别用 `RouterGroup`**：会进 `Routes()` 并参与重复检测；探针/静态/`NoRoute` 走 `GinEngine()`。
8. **注册要在 `Run` 之前、单线程完成**：`Run` 之后再注册会在注册点 panic（`server: route registration must happen before Run: <method> <path>`），并发注册会让路由索引产生数据竞争。
   `Run` 本身只起 goroutine 就返回，主流程要用 `lifecycle.Std().WaitExit()`（或自己的阻塞逻辑）撑住进程。
9. **`server.Recovery` 在链首带来的观测差异**：panic 请求不会进访问日志，也不会进 `request_latency_seconds`
   （`server.Logger` 在 Recovery 内层，panic 展开时跳过了它的收尾逻辑）。这是 D2 的已知变化；
   需要观测 panic 时用 Recovery 的 `panic recovered` 日志（需要计数可自行加 counter）。
10. **关闭 `server.LoginStatus` 后业务代码仍直接调用 `runtime.LoginStatus(ctx)`**：运行期 panic（被 `server.Recovery` 兜成 500）。框架自身的 `Inject` 已做兼容（无登录态时跳过），但业务 handler 里的 `runtime.LoginStatus` 仍需自行改为自己的身份取值方式；启动日志会列出被禁项。
11. **`Routes()` 只含业务路由**：`GinEngine()` 直挂的路由不在清单里；业务路由对账以 `Routes()` 为准，迁移前后全量对拍用 `GinEngine().Routes()`。
