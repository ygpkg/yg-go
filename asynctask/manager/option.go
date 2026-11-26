package manager

import (
	"context"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// Notifier 是通知器形态下「发布唤醒信号 + 消费唤醒信号」的接口（poll+notify 形态）。
//
// 语义（见 internals 设计「通知器」节）：
//   - 通知器只负责「尽快叫醒执行侧去 Fetch 一次」，不承载业务数据、不参与正确性：
//     信号丢了，Worker 的兜底轮询会补上；Redis 挂了，Notify 失败 Submit 返回
//     ErrNotify（任务已落库），整体退回纯轮询形态。
//   - 队列名 <prefix>:<task_type>（默认 gasync:invoice:issue），message 只带 task_type。
//
// 依赖边界（A10）：asynq 等具体队列实现只在独立子包中出现；manager 主包只持有本接口
// 与空实现，不 import 任何队列库。本期不移植 manager/notifier/asynq 子包，接入方自带实现。
type Notifier interface {
	// Notify 发布一条唤醒信号；taskType 决定队列 <prefix>:<task_type>。
	// 失败（如 Redis 不可用）返回错误；调用方（Submit）据此返回 asynctask.ErrNotify，
	// 任务已落库、勿重试提交，兜底轮询会接手。
	Notify(ctx context.Context, taskType string) error
	// Consume 消费 types 各类型的唤醒信号，收到某类型的信号时调用 wake(taskType)，
	// 让执行侧去 Fetch 一次。通常起一个后台消费者（asynq server）。阻塞或非阻塞由实现决定。
	Consume(ctx context.Context, types []string, wake func(ctx context.Context, taskType string)) error
	// Close 停止消费并释放连接。
	Close() error
}

// Option 是 New 的构造选项。封闭接口（apply 未导出）：签名变更不影响调用方。
type Option interface {
	apply(*newOptions)
}

// newOptions 是构造期选项载体：内嵌 Config（保持「选项写进 Config」的既有语义），
// 另加不可配置的外部依赖（notifier）。
type newOptions struct {
	*Config
	notifier Notifier
}

type optionFunc func(*newOptions)

func (f optionFunc) apply(o *newOptions) { f(o) }

// WithNotifier 注入通知器（poll+notify 形态）。未注入时为纯 poll 形态。
func WithNotifier(n Notifier) Option {
	return optionFunc(func(o *newOptions) { o.notifier = n })
}

// WithBackoff 替换退避策略（默认 DefaultBackoff：指数 5s 起、上限 10m、抖动 ±20%）。
func WithBackoff(f Backoff) Option {
	return optionFunc(func(o *newOptions) {
		if f != nil {
			o.Backoff = f
		}
	})
}

// WithPollInterval 覆盖纯轮询周期。
func WithPollInterval(d time.Duration) Option {
	return optionFunc(func(o *newOptions) { o.PollInterval = d })
}

// WithBatchSize 覆盖单次认领上限。
func WithBatchSize(n int) Option {
	return optionFunc(func(o *newOptions) { o.BatchSize = n })
}

// WithLeaseTTL 覆盖租约时长（续租周期取 1/3）。
func WithLeaseTTL(d time.Duration) Option {
	return optionFunc(func(o *newOptions) { o.LeaseTTL = d })
}

// WithRetention 覆盖终态记录保留时长（0 不清理）。
func WithRetention(d time.Duration) Option {
	return optionFunc(func(o *newOptions) { o.Retention = d })
}

// WithCancelNotifyChannel 开启取消通知：Cancel 在状态翻转事务内 pg_notify 该通道（PG 方言）。
//
// 通道名要与执行侧 asynctask/cancelwatch 的配置一致；空串表示关闭（默认）。
func WithCancelNotifyChannel(channel string) Option {
	return optionFunc(func(o *newOptions) { o.CancelNotifyChannel = channel })
}

// WithReclaimHook 注入失权回收结算钩子（下游在同一事务里按预留额结清阶段预算）。
func WithReclaimHook(hook asynctask.RunReclaimHook) Option {
	return optionFunc(func(o *newOptions) {
		if hook != nil {
			o.ReclaimHook = hook
		}
	})
}
