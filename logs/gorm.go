package logs

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	glogger "gorm.io/gorm/logger"
)

func GetGorm(lgrName string) glogger.Interface {
	lw := loggers[lgrName]
	if lw == nil {
		panic(fmt.Errorf("not found gorm logger %q", lgrName))
	}

	return &gormLogger{
		l: lw.logger.Desugar().WithOptions(zap.AddCallerSkip(2)).Sugar(),
	}
}

var _ glogger.Interface = (*gormLogger)(nil)

type gormLogger struct {
	l *zap.SugaredLogger
}

func (g *gormLogger) LogMode(level glogger.LogLevel) glogger.Interface {
	return g
}

func (g *gormLogger) Info(_ context.Context, msg string, data ...interface{}) {
	Infof(msg, data...)
}

func (g *gormLogger) Warn(_ context.Context, msg string, data ...interface{}) {
	Warnf(msg, data...)
}

func (g *gormLogger) Error(_ context.Context, msg string, data ...interface{}) {
	Errorf(msg, data...)
}

func (g *gormLogger) Trace(_ context.Context, begin time.Time, fc func() (string, int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()
	if err != nil {
		g.l.With(
			zap.String("elapsed", fmt.Sprintf("%vms", elapsed.Nanoseconds()/1e6)),
			zap.Int64("rows", rows),
			zap.Error(err),
		).Warn(sql)
	} else {
		g.l.With(
			zap.String("elapsed", fmt.Sprintf("%vms", elapsed.Nanoseconds()/1e6)),
			zap.Int64("rows", rows),
		).Debug(sql)
	}
}
