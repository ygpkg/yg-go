package manager

import (
	"math/rand"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// Config 任务中心配置。字段与契约「配置与默认值」表逐行对齐。
//
// 归属 manager 的项（PollInterval / FallbackPollInterval / BatchSize / LeaseTTL /
// ReaperInterval / CleanupInterval / DefaultMaxAttempts / DefaultTimeout /
// MaxPayloadBytes / Retention / Backoff / NotifierQueuePrefix）在此定义；
// worker 侧共享的 Concurrency / ShutdownTimeout 默认值见契约表，由 worker 包自行持有。
type Config struct {
	// PollInterval 已废弃：轮询由 worker.WorkerConfig.PollInterval 控制，本字段不生效
	// （构造函数仍会校验 < 100ms 报 ErrInvalidConfig，仅为兼容旧配置）。
	//
	// Deprecated: 用 worker.WorkerConfig.PollInterval。
	PollInterval time.Duration
	// FallbackPollInterval 已废弃：兜底轮询由 worker.WorkerConfig.FallbackPollInterval 控制。
	//
	// Deprecated: 用 worker.WorkerConfig.FallbackPollInterval。
	FallbackPollInterval time.Duration
	// BatchSize 是后台维护循环（回收/清理）的单批处理行数，默认 50。
	BatchSize          int
	LeaseTTL           time.Duration // 默认 60s（续租周期取 1/3）
	ReaperInterval     time.Duration // 默认 30s（回收器周期）
	CleanupInterval    time.Duration // 默认 1h；0 关闭
	DefaultMaxAttempts int           // 默认 3（任务未显式指定时）
	DefaultTimeout     time.Duration // 默认 30s（任务未显式指定时）
	MaxPayloadBytes    int           // 默认 4096（超过报 ErrPayloadTooLarge，不截断）
	// MaxResultBytes 结果快照（core_async_task.result）字节上限，默认 4096。
	// 超限按前缀截断、不报错；需要更完整快照的调用方显式调大（如 16 KiB），
	// 并自行保证 result 编码不超预算（本项不做校验，只决定截断位置）。
	MaxResultBytes int
	Retention      time.Duration // 默认 30 天（终态记录保留时长；0 不清理）
	// FailFastChain 为 true 时，任务链某 step 进入失败终态后，同 chain 的后续 step 一并置 canceled，
	// 避免后续步骤因前序永不 succeeded 而永久 pending 积压。默认 false（不级联，保持独立任务语义）。
	FailFastChain       bool
	Backoff             asynctask.Backoff // 默认指数：5s 起、上限 10m、抖动 ±20%；可替换
	NotifierQueuePrefix string            // 默认 "gasync"（<prefix>:<task_type>）

	// CancelNotifyChannel 是取消通知的 PG 通道名（可选，默认空=不发通知）。
	//
	// 语义（设计 §9.1）：Cancel 在**翻转状态的同一个事务内**执行
	// `SELECT pg_notify(<channel>, <json>)`，执行侧的常驻监听器（asynctask/cancelwatch）
	// 收到后按 (task_id, generation, attempt) 取消对应的在途执行，把取消感知从
	// 「等一个续租周期（20s）」压到「网络往返」。
	//
	// 与唤醒（Notifier/WakeSource）是两条独立通道：唤醒只回答「有没有新任务」，
	// 取消只回答「这次执行要停」，不要复用同一个通道（两者消费方与去重口径都不同）。
	//
	// 只在 PostgreSQL 方言下生效（MySQL/SQLite 无 LISTEN/NOTIFY）；
	// 留空表示保持旧行为（只翻转状态，靠续租/补查发现）。
	CancelNotifyChannel string

	// ReclaimHook 是「失权回收结算」钩子（可选）：回收器关闭一次失权执行时在同一事务内回调，
	// 供下游按预留额结清阶段预算（设计 §9.2）。实现必须幂等；为空保持旧行为。
	ReclaimHook asynctask.RunReclaimHook
}

// defaultConfig 返回带契约默认值的 Config。
func defaultConfig() *Config {
	return &Config{
		PollInterval:         500 * time.Millisecond,
		FallbackPollInterval: 30 * time.Second,
		BatchSize:            50,
		LeaseTTL:             60 * time.Second,
		ReaperInterval:       30 * time.Second,
		CleanupInterval:      time.Hour,
		DefaultMaxAttempts:   3,
		DefaultTimeout:       30 * time.Second,
		MaxPayloadBytes:      4096,
		MaxResultBytes:       4096,
		Retention:            30 * 24 * time.Hour,
		Backoff:              DefaultBackoff,
		NotifierQueuePrefix:  "gasync",
	}
}

// Backoff 计算第 attempt 次失败后的退避时长。可替换：Config.Backoff 或 WithBackoff 注入。
//
// attempt 从 1 开始（第 1 次失败后等待的时长）。实现需对抖动后的结果做下界保护，
// 避免负数或过小的退避。
// Backoff 类型已下沉到根包 asynctask，此别名仅为向后兼容旧引用。
type Backoff = asynctask.Backoff

// DefaultBackoff 是默认退避策略：指数退避，5s 起、上限 10m、抖动 ±20%。
//
// 序列（无抖动时）：5s、10s、20s、40s、80s、160s、320s、600s（封顶）。
var DefaultBackoff asynctask.Backoff = func(attempt int) time.Duration {
	return exponentialBackoff(attempt, 5*time.Second, 10*time.Minute, 0.2)
}

// exponentialBackoff 实现 base*2^(attempt-1) 的指数退避，封顶 cap，并施加 ±jitter 抖动。
//
// attempt <= 0 视为第 1 次；抖动后的结果不低于 1ms。
func exponentialBackoff(attempt int, base, cap time.Duration, jitter float64) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	d := base
	for i := 1; i < attempt; i++ {
		if d >= cap {
			break
		}
		d *= 2
	}
	if d > cap {
		d = cap
	}
	if jitter > 0 {
		// 在 [1-jitter, 1+jitter] 区间内均匀抖动。
		factor := 1 + jitter*(2*rand.Float64()-1)
		d = time.Duration(float64(d) * factor)
	}
	if d < time.Millisecond {
		d = time.Millisecond
	}
	return d
}
