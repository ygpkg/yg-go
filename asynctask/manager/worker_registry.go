package manager

import (
	"context"

	sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"
)

// WorkerRegistry 是「worker 存活 / 在线数」判定接口（可选能力）。
//
// 默认实现走租约推导（lease_until > now 的 worker_id 视为在线），零新增表、零外部依赖。
// 需要「主动心跳/精确在线数」的场景可由接入方另接实现（本期不移植 manager/health/redis 子包）。
type WorkerRegistry interface {
	// IsWorkerAlive 判断某 worker 最近是否活跃。
	IsWorkerAlive(ctx context.Context, taskType, workerID string) (bool, error)
	// AliveWorkers 返回某任务类型最近活跃的 worker 数。
	AliveWorkers(ctx context.Context, taskType string) (int64, error)
}

// Registry 返回默认的 worker 存活判定器（租约推导）。
func (c *Manager) Registry() WorkerRegistry { return &leaseWorkerRegistryMgr{m: c} }

// leaseWorkerRegistryMgr 通过 Manager 的 db 做查询（core_async_task.lease_until 推导在线）。
type leaseWorkerRegistryMgr struct{ m *Manager }

func (r *leaseWorkerRegistryMgr) IsWorkerAlive(ctx context.Context, taskType, workerID string) (bool, error) {
	// 用方言的「当前时间」表达式比较，避免 Go time.Time 参数在各驱动下的序列化格式差异
	// （sqlite 下 time.Time 参数与存储的 RFC3339 文本字典序不兼容，会导致比较恒假）。
	// 用方言的「当前时间」比较；SQLite 需两侧 julianday 化（lease_until 是 RFC3339 文本，
	// 直接与 datetime('now') 做字典序比较会与时间无关地恒真）。
	q := r.m.db.WithContext(ctx).Model(&sourcedb.CoreAsyncTask{}).
		Where("worker_id = ? AND "+r.m.dialect.ColGT("lease_until"), workerID)
	if taskType != "" {
		q = q.Where("task_type = ?", taskType)
	}
	var cnt int64
	if err := q.Count(&cnt).Error; err != nil {
		return false, err
	}
	return cnt > 0, nil
}

func (r *leaseWorkerRegistryMgr) AliveWorkers(ctx context.Context, taskType string) (int64, error) {
	q := r.m.db.WithContext(ctx).Model(&sourcedb.CoreAsyncTask{}).
		Where("worker_id <> '' AND " + r.m.dialect.ColGT("lease_until"))
	if taskType != "" {
		q = q.Where("task_type = ?", taskType)
	}
	var cnt int64
	// 去重计数不同 worker_id。
	if err := q.Distinct("worker_id").Count(&cnt).Error; err != nil {
		return 0, err
	}
	return cnt, nil
}
