# yg-go

言古科技 web 基础库

[![CI](https://github.com/ygpkg/yg-go/actions/workflows/ci.yml/badge.svg)](https://github.com/ygpkg/yg-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ygpkg/yg-go)](https://goreportcard.com/report/github.com/ygpkg/yg-go)
[![GoDoc](https://godoc.org/github.com/ygpkg/yg-go?status.png)](http://godoc.org/github.com/ygpkg/yg-go)
[![license](https://img.shields.io/badge/license-GPL%20V3.0-blue.svg?maxAge=2592000)](https://github.com/ygpkg/yg-go/blob/master/LICENSE)

## 版本与分支策略

> **`release-1.23` 分支已封板**：该分支进入维护期，**只修 bug，不再新增功能**。新的功能开发、能力演进与结构调整一律落在 `main` 分支。

`main` 分支聚焦 **Go 1.27** 版本，作为后续功能的唯一开发主线。若你依赖的是旧版本行为，请继续使用 `release-1.23`；若需要新特性，请跟进 `main`。

# 使用说明

## 目录结构

* `apis` 基于Gin封装的更易用的，http服务框架，参考 [Web服务开发](#Web服务开发)
* `cache` 缓存相关的工具
* `config` 该包中所有用到的配置的结构体定义，以及配置文件的读取
* `dbutil` 数据库相关的工具, mysql 和 redis
* `encryptor` 加密解密相关的工具
* `filesys` 文件系统相关的工具
* `httptools` http相关的工具
* `lifecycle` 程序生命周期管理相关的工具
* `logs` 日志相关的工具
* `nettools` 网络相关的工具
* `notify` 通知相关的工具, 邮件、短信和微信
* `pool` 池相关的工具，可用于资源池
* `random` 随机相关的工具
* `settings` 程序设置相关的工具，获取数据库中的设置和远程API的设置（公共函数均以 `ctx context.Context` 为首个参数，便于日志/链路追踪）
* `shell` shell相关的工具
* `tests` 测试相关的工具
* `types` 补充的通用数据结构
* `validate` 数据验证相关的工具
* `vendor` go mod vendor 目录
* `wechatmp` 微信小程序相关的工具

## Web服务开发

本框架中定义的接口统一采用`POST`请求，路径为前缀(`/apis/p/`)加方法名(如`/account.CreateRole`)，大小写敏感。
完整请求 如`POST /apis/p/account.CreateRole`

**请求Body**
```json
{
    // cmd 方法名，路径中已经包含，可为空
    "cmd": "account.CreateRole”,
    "env": "环境类型，可为空",

    // 业务内容
    "Request": {
        "name": "",
        "description": "",
        "Offset": 1,
        "Filters": [{
            "Field": "username",
            "Value": ["adm"],
            "ExactMatch": false
        }, {
            "Field": "auto",
            "Value": ["adm"],
            "ExactMatch": false
        }],
    }
}
```

**响应Body**
```json
{
    "code": 10001,
    "message": "错误信息",
    "env": "环境类型，可为空"

    // 业务内容
    "Response": {...}
}
```
`code`为0则是成功，大于0为失败，`code`小于10000时，意义同`http`标准返回码(如400,401,404等)

### 业务错误码

* 10001: ...


### 认证

认证主体采用`JWT`，`token`中为自定义的`claims`，通过`Authorization`头部传递，格式如下：

```http
Authorization: Bearer ${module}-${TOKEN}
```

其中`${module}`为模块名，默认为空，用于区分不同模块或者用户角色系统的`token`，在调用`AuthInject`时，会自动注入`module`。

`${TOKEN}`为`JWT`生成的`token`

可参考 https://github.com/ygpkg/yg-go/blob/main/apis/runtime/server/author.go#L18





## 日志初始化与退出

启动时调用 `logs.ApplyConfig(module, cfg)` 并处理返回的错误。`logs.ConfigError` 提供失败的 logger 组、条目索引和 writer 名。配置失败时，已有 logger 继续可用。应用可决定中止启动、移除可选出口或回落 stdout。

`ApplyConfig` 成功后会关闭被替换的出口。调用前应停止使用被替换组的旧 logger 实例；库内的 gorm / es 适配器（`GetGorm`、`GetESLogger`）在每次写入时重新解析分组，因此会跟随新配置，应用自己缓存的 `*zap.SugaredLogger` 需要重新获取。已有调用方可以继续使用 `ReloadConfig`，它保留配置失败时 panic 的行为，已发出的 logger 实例可用到退出。

应用结束业务工作后调用 `logs.Shutdown(ctx)`。同一批出口只关闭一次，CLS 关闭结果包含 SDK 错误、已入队日志的投递结果和累计投递失败数。排空预算以 `ctx` 的 deadline 为上限（旧 `logs.Close()` 用 500 ms），只覆盖排空阶段，等待阻塞中的日志写入不计入；调用方等待严格受 `ctx` 限制，超时后后台清理仍可能继续。关闭信号会唤醒 CLS SDK 的 linger 等待，正常端点下排空约 100–300 ms。

CLS 配置可显式设置 `tencent_cls.max_block_sec: 0`，发送积压时立即丢弃，并按 2 的幂采样在 stderr 上报累计拒绝数。省略时沿用 5 秒设置。

> 维护注意：`vendor/github.com/tencentcloud/tencentcloud-cls-sdk-go/` 带两处本地补丁（关闭时唤醒 timer、批次记账对称，均以 `yg-go patch:` 标注）。`go mod vendor` 会覆盖它们，`go test ./logs/` 会因此失败，需要按注释重新施加。
