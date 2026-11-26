-- asynctask schema / SQLite 3（**仅用于单元测试**）
--
-- 生产只用 postgres.sql / mysql.sql；本文件让 sqlite 测试与 PG 保持同一套幂等语义
-- （SQLite 支持部分唯一索引，因此不需要 MySQL 的生成列技巧）。
-- 幂等：全部 CREATE ... IF NOT EXISTS。

CREATE TABLE IF NOT EXISTS core_async_task (
  id            varchar(36) NOT NULL,
  created_at    datetime,
  updated_at    datetime,
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
  next_run_at   datetime     NOT NULL,
  lease_until   datetime,
  worker_id     varchar(64)  NOT NULL DEFAULT '',
  started_at    datetime,
  finished_at   datetime,
  duration_ms   bigint       NOT NULL DEFAULT 0,
  last_error    text,
  result        text,
  request_id    varchar(64)  NOT NULL DEFAULT '',
  trace_id      varchar(64)  NOT NULL DEFAULT '',
  labels        text,
  PRIMARY KEY (id)
);

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
  created_at     datetime,
  async_task_id  varchar(36) NOT NULL,
  generation     integer     NOT NULL DEFAULT 1,
  attempt        integer     NOT NULL,
  status         varchar(16) NOT NULL,
  failure_kind   varchar(16) NOT NULL DEFAULT '',
  worker_id      varchar(64) NOT NULL DEFAULT '',
  started_at     datetime    NOT NULL,
  finished_at    datetime,
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
  enabled     boolean      NOT NULL DEFAULT 1,
  updated_at  datetime,
  PRIMARY KEY (id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_core_async_task_type_task_type
  ON core_async_task_type (task_type);
