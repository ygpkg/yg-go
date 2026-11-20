# AGENTS.md

本文件约定 yg-go 仓库的通用协作规范，人与 AI 协作时均需遵守。

## 版本 / Tag 约定

- 本仓库的 git tag **跟随 Go 语言大版本**，格式 `v<go-major>.<go-minor>.<patch>`：
  - `go 1.23` 时期的 tag 形如 `v1.23.x`；
  - `go 1.27`（当前 `go.mod` 为 `go 1.27.4`）时期的 tag 形如 `v1.27.x`。
- 每次升级 Go 大版本时，tag 的 major.minor 同步切换到对应 Go 版本；patch 位在**同一 Go 版本内递增**。
- 发版时 patch 位 +1（例如当前 `v1.27.1` → 下一个 `v1.27.2`）。**破坏性 API 变更也走 patch 位递增**（沿用本仓既有口径），但必须在 release note 中显著标注破坏性变更与迁移指引。
- **tag 只能打在 `main` 或 `release/*` 分支上**：tag 指向的提交必须是这两类分支上的提交，**不得**打在 `feat/*`、`fix/*`、`chore/*`、`dev/*` 等工作分支上。
  - 反例（2026-11-20）：`v1.27.8` 打在 `feat/worker-result-handler` 上，`main` 落后于该 tag；随后从 `main` 发出的 `v1.27.9` 静默丢掉了已发布的 `ResultHandler` / `ResultProvider`，下游升上去直接编译失败，只能再补 `v1.27.10`。
  - 在 `release/*` 上打 tag 时，该分支必须在发版后合回 `main`，保证 `main` 永不落后于任何已发布 tag。
- 发版前自检（任一不通过就停，不得打 tag）：
  ```bash
  git fetch origin --tags
  git log --oneline origin/main..<tag>   # 从 main 发版时必须为空
  git branch -r --contains <tag>         # 只应命中 origin/main 或 origin/release/*
  ```
- 升级 Go 版本、或跨 major.minor 调整 tag 前需与维护者确认。

## 日志 / Context 约定

- 需要日志/追踪的公共函数，`ctx context.Context` 一律作为**第一个参数**，不得用可选 Option 或 `*WithCtx` 后缀重载替代。
- 日志统一走 `logs.XxxContext(ctx, ...)` / `logs.XxxContextf(ctx, ...)`，确保 reqid 与上下文字段随调用链传递。
- DB 操作统一 `db := dbtools.Core().WithContext(ctx)`，使 gorm 日志能带出 reqid。
- 纯初始化、无请求上下文的路径显式传 `context.Background()`，并在注释说明「初始化路径，无追踪上下文」。

## 待补充

- 后续按需补充提交规范、目录约定、CI 要求等。
