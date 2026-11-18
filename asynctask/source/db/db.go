// Package db 是 asynctask 的「基于数据库的 Source」实现：把任务认领（claim）、续租（lease）、
// 结果保存（save）、回收（reap）与清理（cleanup）落在本地数据库，实现 asynctask.Source。
//
// 依赖边界（A10）：本包只依赖根包 asynctask 与 gorm，不 import manager / worker / asynq。
// 这样：
//   - 简单场景可直接 source/db + worker 组合，不必构造 Manager（不拖管理面）；
//   - 与 source/remote（HTTP 实现）对称，Source 的两种实现平级；
//   - manager 单向依赖本包（管理面读走导出的 Store），不存在循环引用。
package db

import (
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"gorm.io/gorm"
)

// Config 是 DB Source 的构造配置。
type Config struct {
	// DB 写主库句柄（必填）。读写分离下注入从库会让认领读到延迟数据、幂等回查读不到刚插入的行。
	DB *gorm.DB
	// LeaseTTL 租约时长，默认 60s。续租周期由 worker 侧取 1/3。
	LeaseTTL time.Duration
	// Backoff 重试退避函数，默认 asynctask 的指数退避（通过 manager.DefaultBackoff 注入；为空时用内置默认）。
	Backoff asynctask.Backoff
	// WorkerID 本执行节点标识，写入认领行的 worker_id 列；空则生成 "asynctask-db-<uuid>"。
	WorkerID string
	// FailFastChain 为 true 时，任务链某 step 失败终态后同 chain 后续 step 置 canceled。
	FailFastChain bool
}

// Source 导出类型别名，便于调用方持有具体类型；其方法集满足 asynctask.Source。
type Source = source

// New 构造一个 DB Source。DB 为 nil 返回 asynctask.ErrNilDB 包装错误。
//
// 注意：本函数不建表。表结构真源是 asynctask/ddl 的 SQL 文件，由应用的迁移流程执行一次；
// 本包与 manager 都不拥有 DDL 入口（避免多处 DDL 源头），SchemaModels 由 manager 转发。
func New(cfg Config) (*Source, error) {
	if cfg.DB == nil {
		return nil, asynctask.ErrNilDB
	}
	if cfg.LeaseTTL <= 0 {
		cfg.LeaseTTL = 60 * time.Second
	}
	if cfg.Backoff == nil {
		cfg.Backoff = defaultBackoff
	}
	if cfg.WorkerID == "" {
		cfg.WorkerID = "asynctask-db-" + genUUID()
	}
	dialect := DetectDialect(cfg.DB.Dialector.Name())
	return &source{
		store:         newStore(cfg.DB, dialect, cfg.LeaseTTL),
		db:            cfg.DB,
		dialect:       dialect,
		backoff:       cfg.Backoff,
		workerID:      cfg.WorkerID,
		failFastChain: cfg.FailFastChain,
	}, nil
}

// defaultBackoff 是 Backoff 未注入时的兜底：指数退避，5s 起、上限 10m、无抖动。
// 调用方一般应注入 manager.DefaultBackoff（带抖动）以获得更平滑的重试分布。
func defaultBackoff(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := 5 * time.Second
	for i := 1; i < attempt; i++ {
		if d >= 10*time.Minute {
			break
		}
		d *= 2
	}
	if d > 10*time.Minute {
		d = 10 * time.Minute
	}
	return d
}

// Store 返回本 Source 的只读查询接口（供管理面或外部读取任务/尝试记录）。
func (s *source) Store() Store { return s.store }
