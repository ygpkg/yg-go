# yg-go

言古科技 web 基础库

[![CI](https://github.com/ygpkg/yg-go/actions/workflows/ci.yml/badge.svg)](https://github.com/ygpkg/yg-go/actions/workflows/ci.yml)
[![Go Report Card](https://goreportcard.com/badge/github.com/ygpkg/yg-go)](https://goreportcard.com/report/github.com/ygpkg/yg-go)
[![GoDoc](https://godoc.org/github.com/ygpkg/yg-go?status.png)](http://godoc.org/github.com/ygpkg/yg-go)
[![license](https://img.shields.io/badge/license-GPL%20V3.0-blue.svg?maxAge=2592000)](https://github.com/ygpkg/yg-go/blob/master/LICENSE)

# 使用说明

## 目录结构

* `apis` 基于Gin封装的更易用的，http服务框架
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
