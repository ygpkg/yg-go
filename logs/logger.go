package logs

import (
	"fmt"
	"os"
	"sync"

	"github.com/ygpkg/yg-go/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerWrapper struct {
	module, name string
	core         zapcore.Core
	logger       *zap.SugaredLogger
}

var (
	loggers = map[string]*loggerWrapper{
		"default": &loggerWrapper{name: "default"},
	}
	loggersLock = new(sync.RWMutex)

	stdcfg = zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "lvl",
		NameKey:        "mod",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	accessCfg = zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "lvl",
		NameKey:        "reqid",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("01-02T15:04:05.000"),
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	logger *zap.SugaredLogger

	// accessCore = zapcore.NewTee(zapcore.NewCore(
	// 	zapcore.NewJSONEncoder(accessCfg),
	// 	zapcore.Lock(os.Stdout),
	// 	zap.NewAtomicLevelAt(zapcore.InfoLevel)))
	// accessLogger = zap.New(accessCore, zap.AddCaller(), zap.AddCallerSkip(2)).Sugar()
)

func init() {
	loggersLock.Lock()
	lg := loggers["default"]
	lg.load("-", []config.LogConfig{})
	logger = lg.logger
	loggersLock.Unlock()
}

func Get(lgr string) *zap.SugaredLogger {
	loggersLock.RLock()
	defer loggersLock.RUnlock()
	lw := loggers[lgr]
	if lw == nil {
		panic(fmt.Errorf("not found logger %q", lgr))
	}
	return lw.logger
}

// ReloadConfig .
func ReloadConfig(module string, cfg config.LogsConfig) error {
	loggersLock.Lock()
	defer loggersLock.Unlock()
	for name, cfgs := range cfg {
		if lw, ok := loggers[name]; !ok {
			loggers[name] = newLoggerWrapper(module, name, cfgs)
		} else {
			lw.load(module, cfgs)
		}
	}
	logger = loggers["default"].logger
	return nil
}

func Desugar() *zap.Logger { return logger.Desugar() }
func Named(name string) *zap.SugaredLogger {
	return zap.New(getDefaultLoggerWrapper().core, zap.AddCaller(), zap.AddCallerSkip(0)).Sugar().Named(name)
}
func RequestLogger(reqid string) *zap.SugaredLogger {
	lw := loggers["access"]
	if lw == nil {
		panic(fmt.Errorf("not found logger (access)"))
	}
	return lw.logger.Named(reqid)
}
func With(args ...interface{}) *zap.SugaredLogger     { return logger.With(args...) }
func Sync() error                                     { return logger.Sync() }
func Debug(args ...interface{})                       { logger.Debug(args...) }
func Warn(args ...interface{})                        { logger.Warn(args...) }
func Error(args ...interface{})                       { logger.Error(args...) }
func Fatal(args ...interface{})                       { logger.Fatal(args...) }
func Debugf(template string, args ...interface{})     { logger.Debugf(template, args...) }
func Infof(template string, args ...interface{})      { logger.Infof(template, args...) }
func Warnf(template string, args ...interface{})      { logger.Warnf(template, args...) }
func Errorf(template string, args ...interface{})     { logger.Errorf(template, args...) }
func Fatalf(template string, args ...interface{})     { logger.Fatalf(template, args...) }
func Debugw(msg string, keysAndValues ...interface{}) { logger.Debugw(msg, keysAndValues...) }
func Infow(msg string, keysAndValues ...interface{})  { logger.Infow(msg, keysAndValues...) }
func Warnw(msg string, keysAndValues ...interface{})  { logger.Warnw(msg, keysAndValues...) }
func Errorw(msg string, keysAndValues ...interface{}) { logger.Errorw(msg, keysAndValues...) }
func Fatalw(msg string, keysAndValues ...interface{}) { logger.Fatalw(msg, keysAndValues...) }

func newLoggerWrapper(module, name string, cfgs []config.LogConfig) *loggerWrapper {
	lw := &loggerWrapper{
		module: module,
		name:   name,
	}
	lw.load(module, cfgs)
	return lw
}

// loggerWrapper
func (lw *loggerWrapper) load(module string, cfgs []config.LogConfig) {
	lw.module = module
	if len(cfgs) == 0 {
		cfgs = []config.LogConfig{
			{},
		}
	}
	cores := make([]zapcore.Core, 0, len(cfgs))
	for _, lcfg := range cfgs {
		edr := getLoggerEncoder(lcfg.Encoder)
		lvl := zap.NewAtomicLevelAt(lcfg.Level)
		var syncer zapcore.WriteSyncer
		switch lcfg.Writer {
		case "file":
			syncer = zapcore.AddSync(lcfg.Logger)
		case "workwx":
			wxLgr := NewWorkwxSyncer(lcfg.Key)
			syncer = zapcore.AddSync(wxLgr)
		case "console", "stdout", "":
			syncer = zapcore.Lock(os.Stdout)
		case "aliyunsls", "aliyun", "sls":
			slsLgr := NewAliyunSlsSyncer(*lcfg.AliyunSLS)
			syncer = zapcore.AddSync(slsLgr)
		default:
			panic(fmt.Errorf("unsupport logger writer (%s)", lcfg.Encoder))
		}
		core := zapcore.NewCore(edr, syncer, lvl)
		cores = append(cores, core)
	}

	opts := []zap.Option{zap.AddCaller(), zap.AddCallerSkip(1)}
	fields := []zap.Field{zap.String("module", lw.module)}
	if lw.name != "default" {
		fields = append(fields, zap.String("logger", lw.name))
	}
	opts = append(opts, zap.Fields(fields...))

	lw.core = zapcore.NewTee(cores...)
	lw.logger = zap.New(lw.core, opts...).Sugar()
}

func getLoggerEncoder(edr string) zapcore.Encoder {
	switch edr {
	case "access":
		return zapcore.NewJSONEncoder(accessCfg)
	case "", "std", "default":
	default:
		panic(fmt.Errorf("unsupport logger encoder (%s)", edr))
	}
	return zapcore.NewJSONEncoder(stdcfg)
}

func getDefaultLoggerWrapper() *loggerWrapper {
	return loggers["default"]
}

func SetLevel(lvl zapcore.Level) {
	core := zapcore.NewTee(zapcore.NewCore(
		zapcore.NewJSONEncoder(stdcfg),
		zapcore.Lock(os.Stdout),
		zap.NewAtomicLevelAt(lvl)))
	logger = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)).Sugar()
}
