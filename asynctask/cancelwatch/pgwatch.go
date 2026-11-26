// Package cancelwatch 提供 asynctask.CancelWatcher 的 PostgreSQL LISTEN/NOTIFY 实现。
//
// 定位（设计 §9.1）：它是「取消快路径」的监听端——
//
//	manager.Cancel（状态翻转事务内 pg_notify）
//	  → 本包 LISTEN 收到通知
//	  → worker 按 (task_id, generation, attempt) 找本地活跃执行表
//	  → 取消那次执行的 ctx
//
// 三条边界：
//   - **只负责快，不负责对**：通知可能丢（连接断、库重启、通道创建晚于取消），
//     正确性由 worker 的定时补查与续租失权兜底；
//   - **一条专用连接**：LISTEN 是会话级状态，必须独占一条连接，
//     不能借用连接池里的连接（否则下次查询可能落到另一条没有 LISTEN 的连接上）；
//   - **与取单唤醒分开**：这里只发取消信号，不做「有新任务」的唤醒（WakeSource 是另一个通道）。
//
// 依赖边界：只用已随 gorm postgres 驱动引入的 pgx（不新增外部依赖），不引 gorm。
package cancelwatch

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/ygpkg/yg-go/asynctask"
)

// 默认参数。
const (
	// DefaultChannel 是取消通知的默认通道名（与 manager.Config.CancelNotifyChannel 对齐）。
	DefaultChannel = "asynctask_cancel"
	// DefaultReconnectDelay 是监听连接断开后的重连间隔。
	DefaultReconnectDelay = 3 * time.Second
	// DefaultBuffer 是信号通道的默认缓冲深度。
	DefaultBuffer = 256
)

// 哨兵错误。
var (
	// ErrInvalidConfig 配置非法（如 DSN 为空）。
	ErrInvalidConfig = fmt.Errorf("%w: cancelwatch invalid config", asynctask.ErrInvalidConfig)
)

// 编译期断言：本包是 asynctask.CancelWatcher 的一种实现。
var _ asynctask.CancelWatcher = (*PGWatcher)(nil)

// Listener 是「一条已 LISTEN 的 PG 连接」的最小抽象。
//
// 抽出来是为了让「重连 + 分发」这段逻辑可以在没有真实 PG 的情况下单测
// （真库测试按仓库规则不在这里跑）。
type Listener interface {
	// Listen 在连接上执行 LISTEN <channel>。
	Listen(ctx context.Context, channel string) error
	// WaitFor 阻塞等待一条通知，返回其载荷；ctx 取消时返回 ctx 的错误。
	WaitFor(ctx context.Context) (payload string, err error)
	// Close 关闭连接。
	Close(ctx context.Context) error
}

// Config 是 PGWatcher 的配置。
type Config struct {
	// DSN 是 PostgreSQL 连接串（必填，除非注入了 Connect）。
	DSN string
	// Channel 是监听通道名，默认 DefaultChannel；必须与 Cancel 侧的配置一致。
	Channel string
	// Logger 可选，默认 asynctask.DefaultLogger()。
	Logger asynctask.Logger
	// ReconnectDelay 是断线重连间隔，默认 DefaultReconnectDelay。
	ReconnectDelay time.Duration
	// Buffer 是信号通道缓冲深度，默认 DefaultBuffer。缓冲满时监听循环阻塞等待消费
	// （不丢取消信号：宁可慢一点，也不能把一次取消静默扔掉）。
	Buffer int
	// Connect 可选：注入连接构造函数（测试用）。nil 时用 pgx 直连 DSN。
	Connect func(ctx context.Context) (Listener, error)
}

// PGWatcher 是 CancelWatcher 的 PG LISTEN 实现。并发安全，Watch 可重复调用（共用同一条监听）。
type PGWatcher struct {
	cfg    Config
	logger asynctask.Logger
	out    chan asynctask.CancelSignal

	mu      sync.Mutex
	started bool
	cancel  context.CancelFunc
	done    chan struct{}
}

// New 构造 PGWatcher；DSN 为空且未注入 Connect 时返回 ErrInvalidConfig。
func New(cfg Config) (*PGWatcher, error) {
	if strings.TrimSpace(cfg.DSN) == "" && cfg.Connect == nil {
		return nil, fmt.Errorf("%w: DSN is required", ErrInvalidConfig)
	}
	if strings.TrimSpace(cfg.Channel) == "" {
		cfg.Channel = DefaultChannel
	}
	if cfg.ReconnectDelay <= 0 {
		cfg.ReconnectDelay = DefaultReconnectDelay
	}
	if cfg.Buffer <= 0 {
		cfg.Buffer = DefaultBuffer
	}
	if cfg.Logger == nil {
		cfg.Logger = asynctask.DefaultLogger()
	}
	if cfg.Connect == nil {
		dsn := cfg.DSN
		cfg.Connect = func(ctx context.Context) (Listener, error) {
			conn, err := pgx.Connect(ctx, dsn)
			if err != nil {
				return nil, err
			}
			return &pgListener{conn: conn}, nil
		}
	}
	return &PGWatcher{
		cfg:    cfg,
		logger: cfg.Logger,
		out:    make(chan asynctask.CancelSignal, cfg.Buffer),
	}, nil
}

// Channel 返回实际监听的通道名（运维排障用）。
func (w *PGWatcher) Channel() string { return w.cfg.Channel }

// Watch 返回取消信号通道，并（首次调用时）**同步**建连 + LISTEN 后启动常驻监听。
//
// 首次调用把建连错误直接返回给调用方：DSN 填错、端口写成 MySQL、通道名非法这类配置错误
// 在启动期就暴露，而不是退化成后台每 3s 一条 Warnw 的静默重试（现场表现只是「取消变慢了」）。
// 失败时不启动监听循环，下一次 Watch 会重新尝试建连。
//
// 为了不重复建连，首次建好的连接直接交给 loop 的第一轮；后续断线重连仍走 cfg.Connect。
//
// 注：首次调用持有 w.mu 完成建连（只发生一次），会短暂阻塞并发的 Watch；
// 建连用的是调用方 ctx（本方法不加自己的超时），需要上限请由调用方在 ctx 上加时限。
//
// ctx 是监听的生命周期：ctx 取消后监听停止，但**通道不关闭**（原因见 Close）。
// 重复调用返回同一个通道；监听随 ctx 结束后不会再重建（一个 watcher 对应一个执行期，
// worker 重启请新建 watcher），取消的正确性仍由 worker 的定时补查兜住。
func (w *PGWatcher) Watch(ctx context.Context) (<-chan asynctask.CancelSignal, error) {
	if w == nil {
		return nil, ErrInvalidConfig
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.started {
		return w.out, nil
	}
	first, err := w.connectAndListen(ctx)
	if err != nil {
		return nil, err
	}
	// 监听循环跟随 Watch 的 ctx：调用方（worker）用它的执行期 ctx，
	// 停机时循环自然退出；Close 再兜一层。
	runCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel
	w.done = make(chan struct{})
	w.started = true
	go w.loop(runCtx, first)
	return w.out, nil
}

// Close 停止监听（幂等）。
//
// 只做 cancel + 有界等循环退出，**不关闭 Watch 返回的信号通道**：监听协程可能正阻塞在
// WaitFor（网络黑洞时 socket 迟迟不返回），此时关闭通道，只要再来一条通知就是
// send on closed channel——那是进程级 panic。消费方（worker 的 consumeCancel）本来就靠
// 自己的 ctx 退出，通道是否关闭不参与正确性，所以这里干脆不关。
//
// 语义：Close 之后通道保持不关闭、也不再有新的分发（取消那一刻已在途的一条可能写进缓冲，
// 无妨）；消费方不要依赖 !ok 判断结束，应依赖自己的 ctx。重复 Close 幂等。
func (w *PGWatcher) Close() error {
	if w == nil {
		return nil
	}
	w.mu.Lock()
	cancel := w.cancel
	done := w.done
	started := w.started
	w.cancel = nil
	w.mu.Unlock()
	if !started {
		return nil
	}
	if cancel != nil {
		cancel()
	}
	if done != nil {
		// 有界等待：上限 min(重连间隔, DefaultReconnectDelay)。重连间隔配得很大时
		// 停机路径也不能跟着等；测试里把重连间隔调小即可快跑。
		wait := w.cfg.ReconnectDelay
		if wait > DefaultReconnectDelay {
			wait = DefaultReconnectDelay
		}
		select {
		case <-done:
		case <-time.After(wait):
			// 连接卡在 WaitFor 时不等太久：Close 是停机路径，不能拖住进程退出。
			// 循环还活着也无妨——通道不关闭，它之后投递只会写进缓冲或被取消打断。
		}
	}
	return nil
}

// loop 是「（复用首次连接或建连）→ LISTEN → 收通知 → 断线重连」的常驻循环。
//
// first 是 Watch 首次同步建好且已 LISTEN 的连接：第一轮直接用它，避免重复建连。
// 重连是必需的：PG 重启、网络抖动、连接被服务端踢掉都会让 LISTEN 静默失效，
// 不重连就等于取消永远收不到（只能等补查）。
func (w *PGWatcher) loop(ctx context.Context, first Listener) {
	defer close(w.done)
	conn := first
	for {
		if ctx.Err() != nil {
			// 建好的连接还没用上就停机：由本函数关闭，避免连接泄漏。
			if conn != nil {
				w.closeListener(conn)
			}
			return
		}
		// listenOnce 负责关闭它本轮使用的连接（first 或自建的那条）。
		err := w.listenOnce(ctx, conn)
		conn = nil
		if err != nil && ctx.Err() == nil {
			w.logger.Warnw(ctx, "asynctask cancel watch connection lost, reconnecting",
				"channel", w.cfg.Channel, "retry_after", w.cfg.ReconnectDelay, "error", err)
		}
		if ctx.Err() != nil {
			return
		}
		if !sleepCtx(ctx, w.cfg.ReconnectDelay) {
			return
		}
	}
}

// connectAndListen 建立一条连接并完成 LISTEN；LISTEN 失败时关掉已建的连接（不留半开连接）。
func (w *PGWatcher) connectAndListen(ctx context.Context) (Listener, error) {
	conn, err := w.cfg.Connect(ctx)
	if err != nil {
		return nil, err
	}
	if err := conn.Listen(ctx, w.cfg.Channel); err != nil {
		w.closeListener(conn)
		return nil, err
	}
	w.logger.Infow(ctx, "asynctask cancel watch listening", "channel", w.cfg.Channel)
	return conn, nil
}

// closeListener 尽力关闭一条连接：与 ctx 解绑并给 3s 上限，停机路径不被卡住的 Close 拖住。
func (w *PGWatcher) closeListener(conn Listener) {
	cctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = conn.Close(cctx)
}

// listenOnce 把一条连接上的通知分发到 out，直到出错或 ctx 结束。
//
// conn 为 nil 表示需要重新建连（重连轮次）；无论成功失败，本轮使用的连接都由本函数关闭。
func (w *PGWatcher) listenOnce(ctx context.Context, conn Listener) error {
	if conn == nil {
		c, err := w.connectAndListen(ctx)
		if err != nil {
			return err
		}
		conn = c
	}
	defer w.closeListener(conn)

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		payload, err := conn.WaitFor(ctx)
		if err != nil {
			return err
		}
		sig, ok := asynctask.DecodeCancelSignal(payload)
		if !ok {
			w.logger.Warnw(ctx, "asynctask cancel signal ignored: bad payload", "payload", payload)
			continue
		}
		// 阻塞投递：缓冲满也不丢取消信号（消费方是 worker 的常驻循环，正常不会积压）。
		// 这里只等 ctx：通道不会被关闭，所以投递永远安全（见 Close）。
		select {
		case w.out <- sig:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// pgListener 是 pgx 连接上的 Listener 实现。
type pgListener struct {
	conn *pgx.Conn
}

// Listen 执行 LISTEN（通道名用 pgx.Identifier 安全引用，避免拼出非法标识符）。
func (l *pgListener) Listen(ctx context.Context, channel string) error {
	_, err := l.conn.Exec(ctx, "LISTEN "+pgx.Identifier{channel}.Sanitize())
	return err
}

// WaitFor 阻塞等一条通知。
func (l *pgListener) WaitFor(ctx context.Context) (string, error) {
	n, err := l.conn.WaitForNotification(ctx)
	if err != nil {
		return "", err
	}
	if n == nil {
		return "", errors.New("cancelwatch: nil notification")
	}
	return n.Payload, nil
}

// Close 关闭连接。
func (l *pgListener) Close(ctx context.Context) error { return l.conn.Close(ctx) }

// sleepCtx 睡 d 或到 ctx 结束；返回 false 表示 ctx 已结束。
func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
