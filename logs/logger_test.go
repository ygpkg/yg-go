package logs

import (
	"testing"

	"go.uber.org/zap/zapcore"
)

func TestLogger(t *testing.T) {
	Debug("Debug: Hello, world")
	Debugf("Debugf: Hello, %s", "world")
	Debugw("Debugw: Hello, world", "key", "value")
	Info("Info: Hello, world")
	Infof("Infof: Hello, %s", "world")
	Infow("Infow: Hello, world", "key", "value")
	Warn("Warn: Hello, world")
	Warnf("Warnf: Hello, %s", "world")
	Warnw("Warnw: Hello, world", "key", "value")
	Error("Error: Hello, world")
	Errorf("Errorf: Hello, %s", "world")
	Errorw("Errorw: Hello, world", "key", "value")
}

func TestLoggerSetLevel(t *testing.T) {
	SetLevel(zapcore.WarnLevel)
	Debug("Debug: Hello, world")
	Debugf("Debugf: Hello, %s", "world")
	Debugw("Debugw: Hello, world", "key", "value")
	Info("Info: Hello, world")
	Infof("Infof: Hello, %s", "world")
	Infow("Infow: Hello, world", "key", "value")
	Warn("Warn: Hello, world")
	Warnf("Warnf: Hello, %s", "world")
	Warnw("Warnw: Hello, world", "key", "value")
	Error("Error: Hello, world")
	Errorf("Errorf: Hello, %s", "world")
	Errorw("Errorw: Hello, world", "key", "value")
}

func TestWithContextLogger(t *testing.T) {
	ctx := WithContextLogger(nil, logger.Named("with-context-logger"))
	DebugContext(ctx, "DebugContext: Hello, world")
	DebugContextf(ctx, "DebugfContextf: Hello, %s", "world")
	InfoContext(ctx, "InfoContext: Hello, world")
	InfoContextf(ctx, "InfoContextf: Hello, %s", "world")
	WarnContext(ctx, "WarnContext: Hello, world")
	WarnContextf(ctx, "WarnContextf: Hello, %s", "world")
	ErrorContext(ctx, "ErrorContext: Hello, world")
	ErrorContextf(ctx, "ErrorContextf: Hello, %s", "world")

	ctx = WithContextFields(ctx, "key", "value")
	DebugContext(ctx, "DebugContext(kv): Hello, world")
	DebugContextf(ctx, "DebugfContextf(kv): Hello, %s", "world")
	InfoContext(ctx, "InfoContext(kv): Hello, world")
	InfoContextf(ctx, "InfoContextf(kv): Hello, %s", "world")
	WarnContext(ctx, "WarnContext(kv): Hello, world")
	WarnContextf(ctx, "WarnContextf(kv): Hello, %s", "world")
	ErrorContext(ctx, "ErrorContext(kv): Hello, world")
	ErrorContextf(ctx, "ErrorContextf(kv): Hello, %s", "world")

	ctx = WithContextFields(ctx, "key2", 123)
	DebugContext(ctx, "DebugContext(kv): Hello, world")
	DebugContextf(ctx, "DebugfContextf(kv): Hello, %s", "world")
	InfoContext(ctx, "InfoContext(kv): Hello, world")
	InfoContextf(ctx, "InfoContextf(kv): Hello, %s", "world")
	WarnContext(ctx, "WarnContext(kv): Hello, world")
	WarnContextf(ctx, "WarnContextf(kv): Hello, %s", "world")
	ErrorContext(ctx, "ErrorContext(kv): Hello, world")
	ErrorContextf(ctx, "ErrorContextf(kv): Hello, %s", "world")
}
