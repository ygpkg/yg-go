package logs

import (
	"context"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type contextKey string

const (
	contextKeyLogger    contextKey = "yg-go-logger"
	contextKeyRequestID contextKey = "reqid"
)

// With 设置日志字段, 返回一个新的日志对象,参数为键值对, 如: With("key", "value")
func With(args ...interface{}) *zap.SugaredLogger { return logger.With(args...) }

// Sync 同步日志缓冲区
func Sync() error { return logger.Sync() }

// Debug 调试
func Debug(args ...interface{}) { logger.Debug(args...) }

// Info 提示信息
func Info(args ...interface{}) { logger.Info(args...) }

// Warn 警告信息
func Warn(args ...interface{}) { logger.Warn(args...) }

// Error 错误信息
func Error(args ...interface{}) { logger.Error(args...) }

// Fatal 致命错误
func Fatal(args ...interface{}) { logger.Fatal(args...) }

// Debugf 调试
func Debugf(template string, args ...interface{}) { logger.Debugf(template, args...) }

// Infof 提示信息
func Infof(template string, args ...interface{}) { logger.Infof(template, args...) }

// Warnf 警告信息
func Warnf(template string, args ...interface{}) { logger.Warnf(template, args...) }

// Errorf 错误信息
func Errorf(template string, args ...interface{}) { logger.Errorf(template, args...) }

// Fatalf 致命错误
func Fatalf(template string, args ...interface{}) { logger.Fatalf(template, args...) }

// Debugw 调试
func Debugw(msg string, keysAndValues ...interface{}) { logger.Debugw(msg, keysAndValues...) }

// Infow 提示信息
func Infow(msg string, keysAndValues ...interface{}) { logger.Infow(msg, keysAndValues...) }

// Warnw 警告信息
func Warnw(msg string, keysAndValues ...interface{}) { logger.Warnw(msg, keysAndValues...) }

// Errorw 错误信息
func Errorw(msg string, keysAndValues ...interface{}) { logger.Errorw(msg, keysAndValues...) }

// Fatalw 致命错误
func Fatalw(msg string, keysAndValues ...interface{}) { logger.Fatalw(msg, keysAndValues...) }

// DebugContext 调试
func DebugContext(ctx context.Context, args ...interface{}) { LoggerFromContext(ctx).Debug(args...) }

// InfoContext 提示信息
func InfoContext(ctx context.Context, args ...interface{}) { LoggerFromContext(ctx).Info(args...) }

// WarnContext 警告信息
func WarnContext(ctx context.Context, args ...interface{}) { LoggerFromContext(ctx).Warn(args...) }

// ErrorContext 错误信息
func ErrorContext(ctx context.Context, args ...interface{}) { LoggerFromContext(ctx).Error(args...) }

// FatalContext 致命错误
func FatalContext(ctx context.Context, args ...interface{}) { LoggerFromContext(ctx).Fatal(args...) }

// DebugContextf 调试
func DebugContextf(ctx context.Context, template string, args ...interface{}) {
	LoggerFromContext(ctx).Debugf(template, args...)
}

// InfoContextf 提示信息
func InfoContextf(ctx context.Context, template string, args ...interface{}) {
	LoggerFromContext(ctx).Infof(template, args...)
}

// WarnContextf 警告信息
func WarnContextf(ctx context.Context, template string, args ...interface{}) {
	LoggerFromContext(ctx).Warnf(template, args...)
}

// ErrorContextf 错误信息
func ErrorContextf(ctx context.Context, template string, args ...interface{}) {
	LoggerFromContext(ctx).Errorf(template, args...)
}

// FatalContextf 致命错误
func FatalContextf(ctx context.Context, template string, args ...interface{}) {
	LoggerFromContext(ctx).Fatalf(template, args...)
}

func DebugContextw(ctx context.Context, msg string, keysAndValues ...interface{}) {
	LoggerFromContext(ctx).Debugw(msg, keysAndValues...)
}

func InfoContextw(ctx context.Context, msg string, keysAndValues ...interface{}) {
	LoggerFromContext(ctx).Infow(msg, keysAndValues...)
}

func WarnContextw(ctx context.Context, msg string, keysAndValues ...interface{}) {
	LoggerFromContext(ctx).Warnw(msg, keysAndValues...)
}

func ErrorContextw(ctx context.Context, msg string, keysAndValues ...interface{}) {
	LoggerFromContext(ctx).Errorw(msg, keysAndValues...)
}

func FatalContextw(ctx context.Context, msg string, keysAndValues ...interface{}) {
	LoggerFromContext(ctx).Fatalw(msg, keysAndValues...)
}

// WithContextFields 设置日志字段上下文
func WithContextFields(ctx context.Context, fields ...interface{}) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	l := logger
	val := ctx.Value(contextKeyLogger)
	if val != nil {
		var ok bool
		l, ok = val.(*zap.SugaredLogger)
		if !ok {
			l = logger
		}
	}
	l = l.With(fields...)
	return context.WithValue(ctx, contextKeyLogger, l)
}

// WithContextLogger 设置日志上下文
func WithContextLogger(ctx context.Context, l *zap.SugaredLogger) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	return context.WithValue(ctx, contextKeyLogger, l)
}

// SetContextFields 设置日志上下文
func SetContextFields(ctx context.Context, fields ...interface{}) {
	if gctx, ok := ctx.(*gin.Context); ok {
		logger := LoggerFromContext(ctx)
		logger = logger.With(fields...)
		gctx.Set(string(contextKeyLogger), logger)
		return
	}
	ctx = WithContextFields(ctx, fields...)
}

// SetContextLogger 设置日志上下文
func SetContextLogger(ctx context.Context, l *zap.SugaredLogger) {
	if gctx, ok := ctx.(*gin.Context); ok {
		gctx.Set(string(contextKeyLogger), l)
		return
	}
	ctx = WithContextLogger(ctx, l)
}

// LoggerFromContext 获取日志上下文
func LoggerFromContext(ctx context.Context) *zap.SugaredLogger {
	if ctx == nil {
		return logger
	}
	if gctx, ok := ctx.(*gin.Context); ok {
		val, ok := gctx.Get(string(contextKeyLogger))
		if !ok {
			return logger
		}
		l, ok := val.(*zap.SugaredLogger)
		if !ok {
			return logger
		}
		return l
	}
	val := ctx.Value(contextKeyLogger)
	if val == nil {
		return logger
	}
	l, ok := val.(*zap.SugaredLogger)
	if !ok {
		return logger
	}
	return l
}
