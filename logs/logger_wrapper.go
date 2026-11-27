package logs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ygpkg/yg-go/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type loggerWrapper struct {
	closers []io.Closer
	logger  *zap.SugaredLogger
}

var (
	configurationGate = make(chan struct{}, 1)
	standaloneClosers []io.Closer
	closing           *shutdownResult
	loggers           = make(map[string]*loggerWrapper)
	loggersLock       sync.RWMutex
	defaultLogger     atomic.Pointer[zap.SugaredLogger]

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
)

func init() {
	lw, _ := buildLogger("-", "default", nil, nil)
	loggers["default"] = lw
	defaultLogger.Store(lw.logger)
}

// Get 获取日志实例
func Get(name string) *zap.SugaredLogger {
	loggersLock.RLock()
	defer loggersLock.RUnlock()
	if lw := loggers[name]; lw != nil {
		return lw.logger
	}
	logger := defaultLogger.Load()
	if name != "" && name != "default" {
		return logger.Named(name)
	}
	return logger
}

// ConfigError identifies the failed writer without exposing its configuration values.
type ConfigError struct {
	Group  string
	Index  int
	Writer string
	Err    error
}

func (e *ConfigError) Error() string {
	return fmt.Sprintf("logger %q entry %d (%q): %v", e.Group, e.Index, e.Writer, e.Err)
}

func (e *ConfigError) Unwrap() error { return e.Err }

// ReloadConfig applies the configuration and panics on configuration errors for compatibility.
func ReloadConfig(module string, cfg config.LogsConfig) error {
	if err := applyConfig(module, cfg, newTencentClsSyncer, true); err != nil {
		panic(err)
	}
	return nil
}

// ApplyConfig updates named logger groups together. Call at startup or after
// stopping use of any logger instances obtained from groups being replaced.
func ApplyConfig(module string, cfg config.LogsConfig) error {
	return applyConfig(module, cfg, newTencentClsSyncer, false)
}

func applyConfig(module string, cfg config.LogsConfig, newCLS func(config.TencentCLSConfig) (managedSyncer, error), keepReplaced bool) error {
	configurationGate <- struct{}{}
	defer func() { <-configurationGate }()
	if closing != nil {
		select {
		case <-closing.done:
		default:
			return errors.New("logs shutdown in progress")
		}
	}
	names := make([]string, 0, len(cfg))
	for name := range cfg {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for i, entry := range cfg[name] {
			if err := validateLogConfig(entry); err != nil {
				return &ConfigError{name, i, entry.Writer, err}
			}
		}
	}
	pending := make(map[string]*loggerWrapper, len(cfg))
	var created []io.Closer
	for _, name := range names {
		lw, err := buildLogger(module, name, cfg[name], newCLS)
		created = append(created, lw.closers...)
		if err != nil {
			if cleanup := closeWriters(created); cleanup != nil {
				return errors.Join(err, fmt.Errorf("discard new log writers: %w", cleanup))
			}
			return err
		}
		pending[name] = lw
	}
	var retired []io.Closer
	loggersLock.Lock()
	for name, lw := range pending {
		if old := loggers[name]; old != nil {
			retired = append(retired, old.closers...)
		}
		loggers[name] = lw
	}
	defaultLogger.Store(loggers["default"].logger)
	loggersLock.Unlock()
	if len(pending) > 0 {
		closing = nil
	}
	if keepReplaced {
		// Legacy callers may retain loggers after ReloadConfig.
		standaloneClosers = append(standaloneClosers, retired...)
	} else if err := closeWriters(retired); err != nil {
		fmt.Fprintf(os.Stderr, "close replaced log writers: %v\n", err)
	}
	return nil
}

// Desugar 获取底层zap.Logger
func Desugar() *zap.Logger { return defaultLogger.Load().Desugar() }

// Named 获取并设置命名日志实例
func Named(name string) *zap.SugaredLogger { return defaultLogger.Load().Named(name) }

// RequestLogger 接口请求日志 需要配置access日志
func RequestLogger(reqid string) *zap.SugaredLogger { return Get("access").Named(reqid) }

func validateLogConfig(cfg config.LogConfig) error {
	switch cfg.Encoder {
	case "", "std", "default", "access":
	default:
		return errors.New("unsupported encoder")
	}
	switch cfg.Writer {
	case "", "console", "stdout", "workwx":
	case "file":
		if cfg.Logger == nil {
			return errors.New("missing file writer configuration")
		}
	case "tencentcls", "tencent", "cls":
		if cfg.TencentCLS == nil {
			return errors.New("missing tencent_cls")
		}
		return validateTencentCLS(*cfg.TencentCLS)
	default:
		return errors.New("unsupported writer")
	}
	return nil
}

type managedSyncer interface {
	zapcore.WriteSyncer
	io.Closer
}

func buildLogger(module, name string, cfgs []config.LogConfig, newCLS func(config.TencentCLSConfig) (managedSyncer, error)) (*loggerWrapper, error) {
	lw := &loggerWrapper{}
	if len(cfgs) == 0 {
		cfgs = []config.LogConfig{{}}
	}
	cores := make([]zapcore.Core, 0, len(cfgs))
	for i, cfg := range cfgs {
		var syncer zapcore.WriteSyncer
		switch cfg.Writer {
		case "file":
			syncer = zapcore.AddSync(cfg.Logger)
		case "workwx":
			syncer = NewWorkwxSyncer(cfg.Key)
		case "tencentcls", "tencent", "cls":
			writer, err := newCLS(*cfg.TencentCLS)
			if err != nil {
				return lw, &ConfigError{name, i, cfg.Writer, err}
			}
			lw.closers = append(lw.closers, writer)
			syncer = writer
		default:
			syncer = zapcore.Lock(os.Stdout)
		}
		cores = append(cores, zapcore.NewCore(getLoggerEncoder(cfg.Encoder), syncer, zap.NewAtomicLevelAt(cfg.Level)))
	}
	fields := []zap.Field{}
	if module != "-" && name != "" {
		fields = append(fields, zap.String("module", module))
	}
	if name != "default" {
		fields = append(fields, zap.String("logger", name))
	}
	lw.logger = zap.New(zapcore.NewTee(cores...), zap.AddCaller(), zap.AddCallerSkip(1), zap.Fields(fields...)).Sugar()
	return lw, nil
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

// SetLevel 设置默认日志级别
func SetLevel(lvl zapcore.Level) {
	core := zapcore.NewTee(zapcore.NewCore(
		zapcore.NewJSONEncoder(stdcfg),
		zapcore.Lock(os.Stdout),
		zap.NewAtomicLevelAt(lvl)))
	defaultLogger.Store(zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1)).Sugar())
}

type shutdownResult struct {
	done chan struct{}
	err  error
}

const legacyCloseTimeout = 500 * time.Millisecond

// Shutdown switches the global logger to stdout and closes managed writers once.
// A canceled context stops waiting. Writer cleanup can continue in the background.
func Shutdown(ctx context.Context) error {
	select {
	case configurationGate <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	result := closing
	if result == nil {
		result = &shutdownResult{done: make(chan struct{})}
		closing = result
		timeout := legacyCloseTimeout
		if deadline, ok := ctx.Deadline(); ok {
			timeout = max(time.Millisecond, time.Until(deadline))
		}
		writers := standaloneClosers
		standaloneClosers = nil
		loggersLock.Lock()
		for _, lw := range loggers {
			writers = append(writers, lw.closers...)
		}
		fallback, _ := buildLogger("-", "default", nil, nil)
		loggers = map[string]*loggerWrapper{"default": fallback}
		defaultLogger.Store(fallback.logger)
		loggersLock.Unlock()
		if len(writers) == 0 {
			close(result.done)
		} else {
			go func() {
				result.err = closeWritersWithin(writers, timeout)
				close(result.done)
			}()
		}
	}
	<-configurationGate
	select {
	case <-result.done:
		return result.err
	default:
	}
	select {
	case <-result.done:
		return result.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func closeWriters(writers []io.Closer) error {
	return closeWritersWithin(writers, legacyCloseTimeout)
}

type timedCloser interface {
	closeWithin(time.Duration) error
}

func closeWritersWithin(writers []io.Closer, timeout time.Duration) error {
	results := make(chan error, len(writers))
	for _, writer := range writers {
		go func() {
			if timed, ok := writer.(timedCloser); ok {
				results <- timed.closeWithin(timeout)
				return
			}
			results <- writer.Close()
		}()
	}
	var err error
	for range writers {
		if result := <-results; result != nil {
			err = errors.Join(err, result)
		}
	}
	return err
}

// Close closes managed writers using the SDK compatibility timeout.
func Close() {
	if err := Shutdown(context.Background()); err != nil {
		fmt.Fprintf(os.Stderr, "close log writers: %v\n", err)
	}
}
