# MinIO to Tencent COS Migration Design

## Background

The current project uses MinIO (self-hosted, S3-compatible) as the object storage backend.
It is accessed via the `S3Fs` driver with `use_path_style: true`, using a configuration
like:

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

The goal is to migrate to Tencent Cloud COS (公有云) with minimal code changes.

## Approach

**Strategy: Configuration switch + data migration.** Zero business code changes.

Tencent COS supports the S3-compatible protocol. The existing `S3Fs` driver can connect to
COS directly by changing configuration parameters. No new driver, no routing layer,
no double-write logic.

## Key Concept: `use_path_style`

S3 object access has two URL styles:

| Style | `use_path_style` | URL Pattern | Used By |
|-------|------------------|-------------|---------|
| Path-Style | `true` | `https://endpoint/bucket/key` | MinIO |
| Virtual-Hosted-Style | `false` | `https://bucket.endpoint/key` | COS, AWS S3 |

MinIO uses path-style (`use_path_style: true`). COS uses virtual-hosted-style
(`use_path_style: false`). The `S3Fs.GetPublicURL()` already handles this branching
logic (see `s3.go:104-111`), so switching this flag is safe.

## Migration Steps

### Phase 1: Full Data Sync (Online, no downtime)

Use `rclone` to perform an initial full sync from MinIO to COS.

```bash
# Configure rclone remotes:
#   minio-remote: S3 type, path_style=true, endpoint=https://s3.i.yygu.cn:58081
#   cos-remote:   S3 type, path_style=false, endpoint=https://cos.ap-guangzhou.myqcloud.com

rclone sync minio-remote:dotpen-api-test cos-remote:dotpen-api-test \
  --progress \
  --checksum \
  --transfers 32 \
  --checkers 64
```

- `--checksum` ensures data integrity by comparing MD5 hashes
- Can run during normal operation; read-only on MinIO side, no impact

### Phase 2: Maintenance Window (Short Downtime)

1. **Stop write operations** to the application (or redirect to a maintenance page)
2. **Final incremental sync:** Capture any data written between Phase 1 and now
   ```bash
   rclone sync minio-remote:dotpen-api-test cos-remote:dotpen-api-test \
     --progress --checksum
   ```
3. **Update application config** -- change the `s3` section:
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
4. **Restart application** to load the new configuration
5. **Verify** -- Confirm files are accessible via COS URLs

### Phase 3: Post-Migration

- **Keep MinIO read-only** for N days (e.g., 7-14 days) as a fallback
- Monitor COS for errors, access latency, and cost
- After the retention period, decommission the MinIO instance

## Affected Code

- `storage.go:137-147` -- `NewStorage()` already supports the `S3` config branch;
  no code change required
- `config/storage.go:15` -- `S3` field already exists in `StorageConfig`
- `s3.go` -- `S3Fs` driver; no changes needed
- `config/storage.go:77-85` -- `S3StorageConfig` has `UsePathStyle` field

**No business code changes are needed.**

## Non-Goals

- No new storage driver implementation
- No double-write routing layer
- No change to the `Storager` interface or `iUploader` interface
- No change to the multipart upload flow in `uploader.go`

## Rollback Plan

If issues are detected after switching to COS:

1. Revert the config to the original MinIO values
2. Restart the application
3. All data is still intact on MinIO (read-only during migration)

## Verification

1. Upload a test file after the switch -- confirm `PublicURL` is accessible
2. Download existing files via `ReadFile` -- confirm paths resolve correctly
3. Generate presigned URLs for GET and PUT -- confirm they work
4. Run the existing storage unit tests against the COS endpoint
