package lifecycle

import (
	"context"
	"io"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/ygpkg/yg-go/logs"
)

var std *LifeCycle

// LifeCycle 应用程序生命周期
type LifeCycle struct {
	ctx    context.Context
	cancle context.CancelFunc
	chExit chan struct{}

	// exitTimeout 退出过期时间
	exitTimeout time.Duration
	// listenSigs 监听的信号量
	listenSigs []os.Signal

	preExitRun []io.Closer
}

// New .
func New() *LifeCycle {
	ctx, cancle := context.WithCancel(context.Background())
	lc := &LifeCycle{
		ctx:         ctx,
		cancle:      cancle,
		chExit:      make(chan struct{}),
		exitTimeout: time.Second * 15,
		listenSigs:  []os.Signal{syscall.SIGTERM, os.Interrupt},
	}

	return lc
}

// SetSignals 设置监听的信号量
func (l *LifeCycle) SetSignals(sigs ...os.Signal) {
	l.listenSigs = sigs
}

// Context 上下文
func (l *LifeCycle) Context() context.Context {
	return l.ctx
}

// C .
func (l *LifeCycle) C() <-chan struct{} {
	return l.chExit
}

// AddCloseFunc 添加退出任务
func (l *LifeCycle) AddCloseFunc(f func() error) {
	l.AddCloser(newCloserFunc(f))
}

// AddCloser 添加退出任务
func (l *LifeCycle) AddCloser(clr io.Closer) {
	if l.preExitRun == nil {
		l.preExitRun = []io.Closer{clr}
		return
	}
	l.preExitRun = append(l.preExitRun, clr)
}

// SetTimeout 退出超时时间
func (l *LifeCycle) SetTimeout(d time.Duration) {
	l.exitTimeout = d
}

// Exit 强制退出
func (l *LifeCycle) Exit() {
	closeCh(l.chExit)
}

// CancelContext 预退出
func (l *LifeCycle) CancelContext() {
	if l.cancle != nil {
		select {
		case <-l.ctx.Done():
		default:
			l.cancle()
		}
	}
}

// WaitExit .
func (l *LifeCycle) WaitExit() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, l.listenSigs...)

	for {
		select {
		case sig := <-sigChan:
			logs.Infof("catch signal: %v", sig)
			for _, lisSig := range l.listenSigs {
				if lisSig == sig {
					logs.Warnf("^C exit.")
					l.exit()
					return
				}
			}

		case <-l.chExit:
			logs.Warnf("others exit.")
			l.exit()
			return
		}
	}
}

func (l *LifeCycle) exit() {
	if l.exitTimeout < time.Microsecond {
		logs.Warnf("Forced exit.")
		os.Exit(0)
	}
	go func() {
		select {
		case <-time.Tick(l.exitTimeout):
			logs.Warnf("Timeout. Forced exit.")
			os.Exit(1)
		}
	}()

	l.cancle()
	var wg sync.WaitGroup
	wg.Add(len(l.preExitRun))
	for _, v := range l.preExitRun {
		go func(clr io.Closer) {
			defer wg.Done()
			if clr != nil {
				clr.Close()
			}
		}(v)
	}
	wg.Wait()
	time.Sleep(time.Second)
	os.Exit(0)
}

func closeCh(ch chan struct{}) {
	select {
	case <-ch:
	default:
		close(ch)
	}
}

type closerFunc struct {
	f func() error
}

func newCloserFunc(f func() error) io.Closer { return &closerFunc{f: f} }

func (c *closerFunc) Close() error {
	return c.f()
}

// Std ..
func Std() *LifeCycle {
	if std == nil {
		std = New()
	}
	return std
}
