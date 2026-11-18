package manager

import (
	"context"
	"strings"
)

// NotifierQueue 计算某任务类型的唤醒队列名：<prefix>:<task_type>。
//
// prefix 为空时按默认 "gasync" 处理（见 Config.NotifierQueuePrefix）。只传类型不带
// 业务数据，因此「按类型分队列」天然隔离：服务 A 只处理发票、服务 B 只处理通知。
// 两个服务注册同一类型时共享队列，正是期望的水平扩展。
func NotifierQueue(prefix, taskType string) string {
	if strings.TrimSpace(prefix) == "" {
		prefix = "gasync"
	}
	return prefix + ":" + taskType
}

// nopNotifier 是 Notifier 的空实现：Notify 恒成功且不投递任何信号，Consume 立即返回，
// Close 空操作。用于「只想拿一个占位通知器」的场景（例如测试里显式关闭唤醒）。
//
// 注意：它不发信号，因此配了它等于纯兜底轮询——Worker 会在 FallbackPollInterval
// 后自己 Fetch。正确性不受影响（通知器不参与正确性）。
type nopNotifier struct{}

func (nopNotifier) Notify(context.Context, string) error { return nil }

func (nopNotifier) Consume(context.Context, []string, func(context.Context, string)) error {
	return nil
}

func (nopNotifier) Close() error { return nil }

// NopNotifier 返回一个不发任何信号的空通知器。
func NopNotifier() Notifier { return nopNotifier{} }

var _ Notifier = nopNotifier{}
