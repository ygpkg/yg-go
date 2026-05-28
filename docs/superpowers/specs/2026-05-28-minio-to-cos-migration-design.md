# MinIO 迁移至腾讯云 COS 设计方案

## 背景

当前项目使用 MinIO（自建，兼容 S3 协议）作为对象存储后端。
通过 `S3Fs` 驱动以 `use_path_style: true` 的方式访问，配置如下：

```yaml
s3:
  purpose: avatar
  end_point: https://s3.i.yygu.cn:58081
  access_key_id: xxx
  secret_access_key: xxx
  region: cn-xian
  bucket: dotpen-api-test
  use_path_style: true
```

目标是将存储迁移到腾讯云 COS（公有云），业务代码改动最小。

## 方案

**策略：配置切换 + 数据迁移，业务代码零改动。**

腾讯云 COS 支持 S3 兼容协议。现有的 `S3Fs` 驱动只需修改配置参数即可连接到 COS。
无需新增驱动、路由层或双写逻辑。

## 核心概念：`use_path_style`

S3 对象访问有两种 URL 风格：

| 风格 | `use_path_style` | URL 格式 | 使用场景 |
|------|------------------|----------|----------|
| 路径风格 | `true` | `https://endpoint/bucket/key` | MinIO |
| 虚拟主机风格 | `false` | `https://bucket.endpoint/key` | COS、AWS S3 |

MinIO 使用路径风格（`use_path_style: true`），COS 使用虚拟主机风格（`use_path_style: false`）。
`S3Fs.GetPublicURL()` 已处理此分支逻辑（见 `s3.go:104-111`），因此切换此标志是安全的。

## 迁移步骤

### 阶段 1：全量数据同步（在线，无需停服）

使用 `rclone` 从 MinIO 全量同步到 COS。

```bash
# 配置 rclone remote：
#   minio-remote: S3 type, path_style=true, endpoint=https://s3.i.yygu.cn:58081
#   cos-remote:   S3 type, path_style=false, endpoint=https://cos.ap-guangzhou.myqcloud.com

rclone sync minio-remote:dotpen-api-test cos-remote:dotpen-api-test \
  --progress \
  --checksum \
  --transfers 32 \
  --checkers 64
```

- `--checksum` 通过比较 MD5 哈希值确保数据完整性
- 可在正常运行期间执行；对 MinIO 端为只读操作，无影响

### 阶段 2：维护窗口（短时间停服）

1. **停止写入操作**（或重定向到维护页面）
2. **最终增量同步：** 捕获阶段 1 到当前期间产生的新数据
   ```bash
   rclone sync minio-remote:dotpen-api-test cos-remote:dotpen-api-test \
     --progress --checksum
   ```
3. **更新应用配置** -- 修改 `s3` 配置段：
   ```yaml
   s3:
     purpose: avatar
     end_point: https://cos.ap-guangzhou.myqcloud.com
     access_key_id: <COS-SecretId>
     secret_access_key: <COS-SecretKey>
     region: ap-guangzhou
     bucket: dotpen-api-test
     use_path_style: false
   ```
4. **重启应用** 加载新配置
5. **验证** -- 确认文件可通过 COS URL 正常访问

### 阶段 3：迁移后

- **保留 MinIO 只读** N 天（如 7-14 天）作为回退
- 监控 COS 的报错、访问延迟和费用
- 保留期结束后，下线 MinIO 实例

## 涉及代码

- `storage.go:137-147` -- `NewStorage()` 已支持 `S3` 配置分支；无需代码改动
- `config/storage.go:15` -- `StorageConfig` 中已存在 `S3` 字段
- `s3.go` -- `S3Fs` 驱动；无需改动
- `config/storage.go:77-85` -- `S3StorageConfig` 已包含 `UsePathStyle` 字段

**无需任何业务代码改动。**

## 非目标

- 不新增存储驱动
- 不新增双写路由层
- 不改动 `Storager` 接口或 `iUploader` 接口
- 不改动 `uploader.go` 中的分片上传流程

## 回滚方案

如果切换到 COS 后发现问题：

1. 将配置恢复为原始 MinIO 配置
2. 重启应用
3. 所有数据在 MinIO 上仍然完整（迁移期间为只读）

## 验证项

1. 切换后上传测试文件 -- 确认 `PublicURL` 可访问
2. 通过 `ReadFile` 下载已有文件 -- 确认路径正确解析
3. 生成 GET 和 PUT 的预签名 URL -- 确认功能正常
4. 针对 COS 端点运行现有存储单元测试
