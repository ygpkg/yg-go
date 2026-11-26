-- asynctask schema / PostgreSQL 12+；幂等，可重放；本包不执行 DDL。
CREATE TABLE IF NOT EXISTS core_async_task (
  id            varchar(36)  NOT NULL,
  created_at    timestamptz,
  updated_at    timestamptz,
  task_type     varchar(128) NOT NULL,
  status        varchar(16)  NOT NULL DEFAULT 'pending',
  priority      integer      NOT NULL DEFAULT 0,
  payload       text,
  uniq_key      varchar(128),
  biz_type      varchar(64)  NOT NULL DEFAULT '',
  biz_id        varchar(64)  NOT NULL DEFAULT '',
  company_id    bigint       NOT NULL DEFAULT 0,
  subject_kind  varchar(64)  NOT NULL DEFAULT '',
  subject_id    varchar(64)  NOT NULL DEFAULT '',
  chain_id      varchar(36)  NOT NULL DEFAULT '',
  step          integer      NOT NULL DEFAULT 0,
  prev_step     integer      NOT NULL DEFAULT -1,
  generation    integer      NOT NULL DEFAULT 1,
  attempt       integer      NOT NULL DEFAULT 0,
  failure_count integer      NOT NULL DEFAULT 0,
  max_attempts  integer      NOT NULL DEFAULT 3,
  timeout_ms    bigint       NOT NULL DEFAULT 0,
  next_run_at   timestamptz  NOT NULL,
  lease_until   timestamptz,
  worker_id     varchar(64)  NOT NULL DEFAULT '',
  started_at    timestamptz,
  finished_at   timestamptz,
  duration_ms   bigint       NOT NULL DEFAULT 0,
  last_error    text,
  result        text,
  request_id    varchar(64)  NOT NULL DEFAULT '',
  trace_id      varchar(64)  NOT NULL DEFAULT '',
  labels        text,
  PRIMARY KEY (id)
);

-- 增量升级（v1.27.12，执行身份与预算）：已有库补 failure_count 列。
-- PG 支持 ADD COLUMN IF NOT EXISTS，可反复重放；MySQL 8 / SQLite 无该语法，
-- 它们的补列方式见 asynctask/README.md「v1.27.12 起新增列 failure_count」的升级片段。
-- 必须放在 CREATE TABLE 之后：新库先建表（已含该列），再 ALTER 为空操作。
ALTER TABLE core_async_task ADD COLUMN IF NOT EXISTS failure_count integer NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_core_async_task_claim
  ON core_async_task (task_type, status, priority DESC, next_run_at);
CREATE INDEX IF NOT EXISTS idx_core_async_task_status_lease_until
  ON core_async_task (status, lease_until);
CREATE INDEX IF NOT EXISTS idx_core_async_task_status_finished_at
  ON core_async_task (status, finished_at);
CREATE INDEX IF NOT EXISTS idx_core_async_task_task_type_status_created_at
  ON core_async_task (task_type, status, created_at);
CREATE INDEX IF NOT EXISTS idx_core_async_task_biz_type_biz_id
  ON core_async_task (biz_type, biz_id);
CREATE INDEX IF NOT EXISTS idx_core_async_task_chain_id_step
  ON core_async_task (chain_id, step);
CREATE INDEX IF NOT EXISTS idx_core_async_task_company_status_created
  ON core_async_task (company_id, status, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_core_async_task_subject
  ON core_async_task (subject_kind, subject_id, task_type, created_at DESC);

CREATE UNIQUE INDEX IF NOT EXISTS uk_core_async_task_uniq_active
  ON core_async_task (uniq_key)
  WHERE uniq_key IS NOT NULL AND status IN ('pending', 'running');

CREATE TABLE IF NOT EXISTS core_async_task_run (
  id             varchar(36) NOT NULL,
  created_at     timestamptz,
  async_task_id  varchar(36) NOT NULL,
  generation     integer     NOT NULL DEFAULT 1,
  attempt        integer     NOT NULL,
  status         varchar(16) NOT NULL,
  failure_kind   varchar(16) NOT NULL DEFAULT '',
  worker_id      varchar(64) NOT NULL DEFAULT '',
  started_at     timestamptz NOT NULL,
  finished_at    timestamptz,
  duration_ms    bigint      NOT NULL DEFAULT 0,
  result         text,
  error_msg      text,
  request_id     varchar(64) NOT NULL DEFAULT '',
  trace_id       varchar(64) NOT NULL DEFAULT '',
  PRIMARY KEY (id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_core_async_task_run_async_task_id_generation_attempt
  ON core_async_task_run (async_task_id, generation, attempt);
CREATE INDEX IF NOT EXISTS idx_core_async_task_run_created_at
  ON core_async_task_run (created_at);
CREATE INDEX IF NOT EXISTS idx_core_async_task_run_status_started_at
  ON core_async_task_run (status, started_at);
CREATE INDEX IF NOT EXISTS idx_core_async_task_run_request_id
  ON core_async_task_run (request_id);

CREATE TABLE IF NOT EXISTS core_async_task_type (
  id          varchar(36)  NOT NULL,
  task_type   varchar(128) NOT NULL,
  enabled     boolean      NOT NULL DEFAULT true,
  updated_at  timestamptz,
  PRIMARY KEY (id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_core_async_task_type_task_type
  ON core_async_task_type (task_type);
