package cancelwatch

// 本文件用可控的假连接测试「建连失败上抛 + 重连 + 分发 + Close」逻辑；真 PG 的 LISTEN 行为
// 按仓库规则不在单测里跑（DDL/连接类测试只在 Reallib 用例里跑且需显式指名的真库）。

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// fakeListener 是按脚本工作的假 PG 连接：payloads 依次返回通知载荷，通道关闭即代表断线。
type fakeListener struct {
	payloads chan string
	listenOK atomic.Bool
	// listens 记录 Listen 的调用次数：用于断言「首次同步建连只 LISTEN 一次」。
	listens atomic.Int32
	closed  atomic.Int32
}

func newFakeListener(buf int) *fakeListener {
	return &fakeListener{payloads: make(chan string, buf)}
}

func (l *fakeListener) Listen(ctx context.Context, channel string) error {
	l.listenOK.Store(true)
	l.listens.Add(1)
	return nil
}

func (l *fakeListener) WaitFor(ctx context.Context) (string, error) {
	select {
	case p, ok := <-l.payloads:
		if !ok {
			return "", errors.New("connection closed")
		}
		return p, nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (l *fakeListener) Close(ctx context.Context) error {
	l.closed.Add(1)
	return nil
}

// stuckListener 模拟「网络黑洞」：WaitFor 无视 ctx、只在显式放行时返回一条通知。
//
// 真 pgx 也可能长时间卡在 socket 上，Close 的有界等待超时后监听协程仍然活着——
// 这正是「Close 与投递协程抢通道」的现场。
type stuckListener struct {
	release  chan string
	listenOK atomic.Bool
	// waiting 标记已进入 WaitFor：测试必须等它之后再 Close，否则 Close 可能赶在
	// 监听协程开跑之前（那样根本走不到「Close 之后才投递」的现场）。
	waiting atomic.Bool
	closed  atomic.Int32
}

func newStuckListener() *stuckListener {
	return &stuckListener{release: make(chan string, 1)}
}

func (l *stuckListener) Listen(ctx context.Context, channel string) error {
	l.listenOK.Store(true)
	return nil
}

// WaitFor 故意不看 ctx：模拟取消 ctx 也无法立刻唤醒的阻塞等待。
func (l *stuckListener) WaitFor(ctx context.Context) (string, error) {
	l.waiting.Store(true)
	return <-l.release, nil
}

func (l *stuckListener) Close(ctx context.Context) error {
	l.closed.Add(1)
	return nil
}

// recorder 记录连接构造次数，按顺序注入假连接。
//
// next 非 nil 时用于**首次**连接（「Watch 同步建连」那一轮）：返回错误即可构造
// 「首次建连失败」；之后的轮次（重连/再次 Watch）从 conns 依次取。
type recorder struct {
	mu    chan struct{}
	conns []*fakeListener
	next  func() (*fakeListener, error)
	calls atomic.Int32
}

func newRecorder(conns ...*fakeListener) *recorder {
	r := &recorder{mu: make(chan struct{}, 1), conns: conns}
	r.mu <- struct{}{}
	return r
}

func (r *recorder) connect(ctx context.Context) (Listener, error) {
	<-r.mu
	defer func() { r.mu <- struct{}{} }()
	n := int(r.calls.Add(1))
	if n == 1 && r.next != nil {
		conn, err := r.next()
		if err != nil {
			return nil, err
		}
		return conn, nil
	}
	// conns 的 0 基下标：首次走 next 时第 2 次调用对应 conns[0]；否则第 1 次调用对应 conns[0]。
	i := n - 1
	if r.next != nil {
		i = n - 2
	}
	if i < 0 || i >= len(r.conns) {
		return nil, errors.New("no more scripted connections")
	}
	return r.conns[i], nil
}

// TestNewRequiresDSN 未给 DSN 也没注入 Connect 视为配置错误。
func TestNewRequiresDSN(t *testing.T) {
	if _, err := New(Config{}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("New err = %v, want ErrInvalidConfig", err)
	}
	w, err := New(Config{Connect: func(context.Context) (Listener, error) { return nil, errors.New("x") }})
	if err != nil || w == nil {
		t.Fatalf("注入 Connect 时应可构造: %v", err)
	}
	if w.Channel() != DefaultChannel {
		t.Fatalf("默认通道 = %q, want %q", w.Channel(), DefaultChannel)
	}
}

// TestPGWatcherWatchFailsOnFirstConnect 首次同步建连失败：错误直接返回给调用方，
// 且不启动后台监听循环（否则配置错误又会退化成每 3s 一条 Warnw 的静默重试）。
func TestPGWatcherWatchFailsOnFirstConnect(t *testing.T) {
	dialErr := errors.New("dial tcp 127.0.0.1:3306: connect: connection refused")
	conn := newFakeListener(1)
	rec := newRecorder(conn)
	rec.next = func() (*fakeListener, error) { return nil, dialErr }
	w, err := New(Config{
		Channel:        "asynctask_cancel_test",
		Connect:        rec.connect,
		ReconnectDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch, err := w.Watch(ctx)
	if !errors.Is(err, dialErr) {
		t.Fatalf("Watch err = %v, want %v", err, dialErr)
	}
	if ch != nil {
		t.Fatalf("首次建连失败时通道应为 nil，实际 %v", ch)
	}

	// 不启动循环：等一小段时间后不应出现后台重连尝试。
	time.Sleep(60 * time.Millisecond)
	if got := rec.calls.Load(); got != 1 {
		t.Fatalf("Connect 调用次数 = %d, want 1（失败后不应在后台重试）", got)
	}

	// 失败不改变「未启动」状态：下一次 Watch 重新建连，这次成功。
	ch, err = w.Watch(ctx)
	if err != nil {
		t.Fatalf("再次 Watch err = %v, want nil", err)
	}
	if ch == nil {
		t.Fatal("再次 Watch 成功时通道不应为 nil")
	}
	if !conn.listenOK.Load() {
		t.Fatal("再次 Watch 返回时连接应已 LISTEN")
	}
	defer func() { _ = w.Close() }()
}

// TestPGWatcherDeliversSignals Watch 同步完成 LISTEN；收到的合法载荷被解码成取消信号，非法载荷丢弃。
func TestPGWatcherDeliversSignals(t *testing.T) {
	conn := newFakeListener(8)
	rec := newRecorder(conn)
	w, err := New(Config{
		Channel:        "asynctask_cancel_test",
		Connect:        rec.connect,
		ReconnectDelay: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() { _ = w.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := w.Watch(ctx)
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	// 首次建连是同步的：Watch 返回时连接已 LISTEN，且只建连/LISTEN 一次（交给 loop 首轮复用）。
	if !conn.listenOK.Load() {
		t.Fatal("Watch 返回时连接应已 LISTEN")
	}
	if got := conn.listens.Load(); got != 1 {
		t.Fatalf("Listen 次数 = %d, want 1（首次建连不应被 loop 重复建连）", got)
	}
	if got := rec.calls.Load(); got != 1 {
		t.Fatalf("Connect 调用次数 = %d, want 1", got)
	}
	if w.Channel() != "asynctask_cancel_test" {
		t.Fatalf("channel = %q", w.Channel())
	}

	// 合法载荷 → 信号；非法载荷被丢弃（不影响后续）。
	conn.payloads <- "not-a-json"
	conn.payloads <- asynctask.CancelSignal{TaskID: "j1", Generation: 2, Attempt: 3}.Encode()

	select {
	case sig := <-ch:
		if sig.TaskID != "j1" || sig.Generation != 2 || sig.Attempt != 3 {
			t.Fatalf("信号 = %+v", sig)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("未收到取消信号")
	}
}

// TestPGWatcherReconnects 连接断开后自动重连，并继续分发通知（漏通知靠 worker 补查兜底，但重连是必需的）。
func TestPGWatcherReconnects(t *testing.T) {
	first := newFakeListener(1)
	second := newFakeListener(1)
	rec := newRecorder(second)
	// 首次连接（Watch 同步建连那一轮）由 next 提供。
	rec.next = func() (*fakeListener, error) { return first, nil }
	w, err := New(Config{
		Channel:        "asynctask_cancel_test",
		Connect:        rec.connect,
		ReconnectDelay: 10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	defer func() { _ = w.Close() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := w.Watch(ctx)
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	if !first.listenOK.Load() {
		t.Fatal("Watch 返回时首条连接应已 LISTEN")
	}
	if got := rec.calls.Load(); got != 1 {
		t.Fatalf("首次 Watch 后 Connect 调用次数 = %d, want 1", got)
	}

	// 关掉第一条连接 → 触发重连。
	close(first.payloads)
	waitCond(t, func() bool { return second.listenOK.Load() }, 2*time.Second)
	if got := rec.calls.Load(); got != 2 {
		t.Fatalf("重连后 Connect 调用次数 = %d, want 2", got)
	}

	second.payloads <- asynctask.CancelSignal{TaskID: "j2"}.Encode()
	select {
	case sig := <-ch:
		if sig.TaskID != "j2" {
			t.Fatalf("信号 = %+v", sig)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("重连后未收到取消信号")
	}
	if got := first.closed.Load(); got != 1 {
		t.Fatalf("断线的连接应被关闭 %d 次", got)
	}
}

// TestPGWatcherClose Close 停止监听且幂等；通道**不关闭**（消费方靠自己的 ctx 退出）。
func TestPGWatcherClose(t *testing.T) {
	conn := newFakeListener(1)
	rec := newRecorder(conn)
	w, err := New(Config{Connect: rec.connect, ReconnectDelay: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	ch, err := w.Watch(context.Background())
	if err != nil {
		t.Fatalf("watch: %v", err)
	}
	if !conn.listenOK.Load() {
		t.Fatal("Watch 返回时连接应已 LISTEN")
	}

	if err := w.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close 应幂等: %v", err)
	}
	// 循环应已退出并关掉连接（ctx 取消能让 fake 的 WaitFor 返回）。
	waitCond(t, func() bool { return conn.closed.Load() == 1 }, 2*time.Second)
	// 通道不关闭：读不到值、也读不到 !ok。
	select {
	case sig, ok := <-ch:
		if !ok {
			t.Fatal("Close 之后不应关闭通道")
		}
		t.Fatalf("Close 之后不应再收到信号: %+v", sig)
	case <-time.After(100 * time.Millisecond):
	}
	// Close 不应引入重连。
	if got := rec.calls.Load(); got != 1 {
		t.Fatalf("Close 后 Connect 调用次数 = %d, want 1", got)
	}
}

// TestPGWatcherCloseWhileWaitForBlocked Close 在 WaitFor 卡住时有界返回；之后监听协程再投递
// 一条通知不能 panic（旧实现在超时分支里 close(out)，这里就是 send on closed channel）。
//
// panic 发生在后台 goroutine，测试无法 recover；只能靠「进程没崩 + 测试跑完」来验证。
// 旧实现在超时分支里 close(out)，之后投递的 select 与 ctx.Done() 竞争（约五成概率命中 send），
// 故跑多轮提高命中率；同时断言「Close 之后通道不关闭」，命中哪一支都会明确失败。
func TestPGWatcherCloseWhileWaitForBlocked(t *testing.T) {
	const rounds = 20
	conns := make([]*stuckListener, 0, rounds)
	for i := 0; i < rounds; i++ {
		conn := newStuckListener()
		conns = append(conns, conn)
		w, err := New(Config{
			Channel: "asynctask_cancel_test",
			Connect: func(context.Context) (Listener, error) { return conn, nil },
			// 有界等待的上限跟随重连间隔，测试里调小以免跑几秒。
			ReconnectDelay: 20 * time.Millisecond,
		})
		if err != nil {
			t.Fatalf("new: %v", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		ch, err := w.Watch(ctx)
		if err != nil {
			cancel()
			t.Fatalf("watch: %v", err)
		}
		if !conn.listenOK.Load() {
			cancel()
			t.Fatal("Watch 返回时连接应已 LISTEN")
		}
		// 等监听协程真正阻塞在 WaitFor 上，Close 才是「黑洞里的 Close」。
		waitCond(t, conn.waiting.Load, 2*time.Second)

		start := time.Now()
		if err := w.Close(); err != nil {
			cancel()
			t.Fatalf("close: %v", err)
		}
		if d := time.Since(start); d > 2*time.Second {
			cancel()
			t.Fatalf("Close 耗时 %v，应有界返回", d)
		}
		// 放行一条通知：监听协程此时仍活着，旧实现会在这里 send on closed channel 直接崩进程。
		conn.release <- asynctask.CancelSignal{TaskID: "late", Generation: 1, Attempt: 1}.Encode()
		cancel()

		// 投递完协程应退出并关掉连接；若旧 bug 命中，进程在这一步之前已 panic。
		waitCond(t, func() bool { return conn.closed.Load() == 1 }, 2*time.Second)
		// 通道始终不关闭；晚到的一条信号无妨（worker 按 (id,generation,attempt) 匹配，
		// 命中与否都不改变「Close 是停机路径」这一语义）。
		select {
		case _, ok := <-ch:
			if !ok {
				t.Fatalf("第 %d 轮：Close 之后不应关闭通道", len(conns)-1)
			}
		default:
		}
	}
}

// waitCond 轮询等待条件成立。
func waitCond(t *testing.T, cond func() bool, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal("条件未在超时内成立")
}
