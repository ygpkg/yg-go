package manager

import (
	"context"
	"sync"
	"time"

	sourcedb "github.com/ygpkg/yg-go/asynctask/source/db"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 类型级启停（type gate）：Disable/Enable 某任务类型，disabled 期间该类型任务不被认领执行
// （停在 pending，不丢），Enable 后自动恢复。语义：core_async_task_type 无行 = 启用（fail-open）。

// Disable 下线任务类型：写开关表 enabled=false，并刷新本进程 TypeGate 缓存。
// 之后该类型的任务不会被认领（停在 pending），Enable 后恢复。类型无任务/未注册也幂等成功。
func (c *Manager) Disable(ctx context.Context, taskType string) error {
	if err := c.setTypeEnabled(ctx, taskType, false); err != nil {
		return err
	}
	if g := c.gateAdapter(); g != nil {
		g.set(taskType, false)
	}
	return nil
}

// Enable 恢复任务类型：写开关表 enabled=true，并刷新本进程 TypeGate 缓存。已是启用幂等返回 nil。
func (c *Manager) Enable(ctx context.Context, taskType string) error {
	if err := c.setTypeEnabled(ctx, taskType, true); err != nil {
		return err
	}
	if g := c.gateAdapter(); g != nil {
		g.set(taskType, true)
	}
	return nil
}

// IsTypeEnabled 查询某类型是否启用（无行/查询失败=启用）。
func (c *Manager) IsTypeEnabled(ctx context.Context, taskType string) (bool, error) {
	var row sourcedb.CoreAsyncTaskType
	err := c.db.WithContext(ctx).Where("task_type = ?", taskType).Take(&row).Error
	if err == gorm.ErrRecordNotFound {
		return true, nil
	}
	if err != nil {
		return true, nil // fail-open
	}
	return row.Enabled, nil
}

// setTypeEnabled upsert 开关行。
func (c *Manager) setTypeEnabled(ctx context.Context, taskType string, enabled bool) error {
	// 用 map 显式写入，避免 GORM 对 bool 零值(false)套用列默认 true。
	row := map[string]any{"id": genUUID(), "task_type": taskType, "enabled": enabled, "updated_at": time.Now()}
	return c.db.WithContext(ctx).Table(sourcedb.CoreAsyncTaskTypeTableName).
		Clauses(onConflictUpdateType()).
		Create(row).Error
}

// TypeGate 返回一个类型级启停开关的只读适配器（供 worker.New 注入）。
//
// 适配器带本地缓存（TTL=cacheTTL），避免 worker 每轮查库；Disable/Enable 会同步刷新本进程缓存。
// 返回的对象满足 worker.TypeGate 接口（Get 语义，见下方 TypeGate 适配器）。
func (c *Manager) TypeGate(cacheTTL time.Duration) *TypeGateAdapter {
	if cacheTTL <= 0 {
		cacheTTL = 5 * time.Second
	}
	g := newTypeGateAdapter(c.db, cacheTTL)
	c.maintMu.Lock()
	c.gate = g
	c.maintMu.Unlock()
	return g
}

// gateAdapter 返回当前进程的类型开关缓存（可能为 nil，未调用 TypeGate 时）。
func (c *Manager) gateAdapter() *TypeGateAdapter {
	c.maintMu.Lock()
	defer c.maintMu.Unlock()
	return c.gate
}

// ---- 内部实现：开关行 upsert 的冲突子句 ----

func onConflictUpdateType() clause.OnConflict {
	return clause.OnConflict{
		Columns:   []clause.Column{{Name: "task_type"}},
		DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
	}
}

// ---- TypeGate 适配器：worker.TypeGate 的 manager 侧实现（带 TTL 缓存）----

// TypeGateAdapter 是 worker.TypeGate 的实现：查 core_async_task_type + 本地 TTL 缓存。
// worker 只依赖其 Get 语义（见 worker.TypeGate 接口）。
type TypeGateAdapter struct {
	db    *gorm.DB
	ttl   time.Duration
	mu    sync.Mutex
	items map[string]gateEntry
}

type gateEntry struct {
	enabled   bool
	expiresAt time.Time
}

func newTypeGateAdapter(db *gorm.DB, ttl time.Duration) *TypeGateAdapter {
	return &TypeGateAdapter{db: db, ttl: ttl, items: make(map[string]gateEntry)}
}

// set 同步刷新缓存项（Disable/Enable 调用）。
func (a *TypeGateAdapter) set(taskType string, enabled bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.items[taskType] = gateEntry{enabled: enabled, expiresAt: time.Now().Add(a.ttl)}
}

// TypeEnabled 实现 worker.TypeGate：优先缓存，未命中查库并回填；无行/查询失败=fail-open 启用。
func (a *TypeGateAdapter) TypeEnabled(ctx context.Context, taskType string) (bool, error) {
	now := time.Now()
	a.mu.Lock()
	if e, ok := a.items[taskType]; ok && now.Before(e.expiresAt) {
		a.mu.Unlock()
		return e.enabled, nil
	}
	a.mu.Unlock()

	enabled := true // fail-open
	var row sourcedb.CoreAsyncTaskType
	err := a.db.WithContext(ctx).Where("task_type = ?", taskType).Take(&row).Error
	if err == nil {
		enabled = row.Enabled
	} else if err != gorm.ErrRecordNotFound {
		// 查询失败不缓存、按 fail-open 返回启用，下轮重试。
		return true, nil
	}

	a.mu.Lock()
	a.items[taskType] = gateEntry{enabled: enabled, expiresAt: now.Add(a.ttl)}
	a.mu.Unlock()
	return enabled, nil
}
