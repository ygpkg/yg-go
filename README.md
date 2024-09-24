# yg-go

言古科技 web 基础库

[![CI](https://github.com/ygpkg/yg-go/actions/workflows/ci.yml/badge.svg)](https://github.com/ygpkg/yg-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ygpkg/yg-go)](https://goreportcard.com/report/github.com/ygpkg/yg-go)
[![GoDoc](https://godoc.org/github.com/ygpkg/yg-go?status.png)](http://godoc.org/github.com/ygpkg/yg-go)
[![license](https://img.shields.io/badge/license-GPL%20V3.0-blue.svg?maxAge=2592000)](https://github.com/ygpkg/yg-go/blob/master/LICENSE)

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
* `settings` 程序设置相关的工具，获取数据库中的设置和远程API的设置
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



