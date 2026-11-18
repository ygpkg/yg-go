package asynctask

import (
	"context"

	"go.uber.org/zap"

	"github.com/ygpkg/yg-go/logs"
)

// 日志字段常量（与 golib 保持相同字符串值，运维侧日志检索规则不变）。
const (
	logKeyTaskType = "task.type"
	logKeyTaskID   = "task.id"
)

// Logger 是本包所需的最小日志面。
//
// 设计动机：golib 的 gasync 依赖带 ctx 的 glog.Logger 接口；yg-go 的 logs 是
// zap 封装、无 Logger 接口、方法不带 ctx。这里收敛出一个「方法签名带 ctx」的薄接口，
// 既让 worker/manager 的调用点保持与源一致（零改动），又避免它们直接耦合 zap
// （只有本文件 import zap）。
//
// 默认实现（loggerAdapter）委托 yg-go logs；ctx 目前被适配层忽略，未来若要把
// request_id/trace_id 打进日志，可在 adapter 内改用 logs.LoggerFromContext(ctx)。
type Logger interface {
	Debugw(ctx context.Context, msg string, kvs ...any)
	Infow(ctx context.Context, msg string, kvs ...any)
	Warnw(ctx context.Context, msg string, kvs ...any)
	Errorw(ctx context.Context, msg string, kvs ...any)
	// With 返回携带固定 kv 字段的子 Logger。
	With(kvs ...any) Logger
}

// loggerAdapter 用 *zap.SugaredLogger 实现 Logger（当前忽略 ctx）。
type loggerAdapter struct {
	s *zap.SugaredLogger
}

func (l loggerAdapter) Debugw(_ context.Context, msg string, kvs ...any) { l.s.Debugw(msg, kvs...) }
func (l loggerAdapter) Infow(_ context.Context, msg string, kvs ...any)  { l.s.Infow(msg, kvs...) }
func (l loggerAdapter) Warnw(_ context.Context, msg string, kvs ...any)  { l.s.Warnw(msg, kvs...) }
func (l loggerAdapter) Errorw(_ context.Context, msg string, kvs ...any) { l.s.Errorw(msg, kvs...) }
func (l loggerAdapter) With(kvs ...any) Logger                           { return loggerAdapter{s: l.s.With(kvs...)} }

// DefaultLogger 返回基于 yg-go logs 默认 logger 的实现。
//
// 说明：logs.With() 空参即返回默认 *zap.SugaredLogger（其 .With() 无字段时等价 root）。
func DefaultLogger() Logger {
	return loggerAdapter{s: logs.With()}
}

// newAsyncTaskLogger 返回带 task.type / task.id 固定字段的基础 logger。
// 注意：task.run.id 等 ctx extra_keys 需由应用在启动时统一配置到 logs。
func newAsyncTaskLogger(asyncTaskID, taskType string) Logger {
	fields := []any{logKeyTaskType, taskType}
	if asyncTaskID != "" {
		fields = append(fields, logKeyTaskID, asyncTaskID)
	}
	return DefaultLogger().With(fields...)
}
