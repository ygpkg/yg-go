package worker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// 每类型只起一个 fetcher（认领者）+ N 个执行槽。
//
// 语义要点：
//   - **先占槽再认领**：fetcher 必须先拿到一个空闲槽才去 Fetch，避免「已认领、排队等槽」
//     白烧一次 attempt（attempt 在认领时就 +1 了）。
//   - **每类型单点轮询**：无论并发多少，空闲时该类型只有一个 goroutine 在查库，
//     排队长度不会把空闲轮询放大成 N 倍。
//   - **执行期续租**：任务执行中按 LeaseTTL/3 调 Source.Renew；续租失权（ErrLeaseLost）
//     立即取消执行 ctx，避免与接管者双跑。
//   - 是否重试由 Source 决定，Worker 只如实上报 Result 与选择的 callback。
//   - Shutdown 用 Source.Release 主动放弃在途任务（不是续租），宁可重跑也不卡住。
type Worker struct {
	src    asynctask.Source
	cfg    WorkerConfig
	logger asynctask.Logger

	mu        sync.Mutex
	handlers  map[string]*handlerEntry
	running   bool // Run 已开始
	stopped   bool // Shutdown 已执行
	order     []string
	startedAt time.Time

	// 运行期状态，Run/Start 时初始化。
	baseCtx       context.Context // 执行期上下文（随调用方取消）
	baseCancel    context.CancelFunc
	fetchCtx      context.Context // 取任务/等待上下文（Shutdown 先取消它）
	fetchCancel   context.CancelFunc
	fetchStop     atomic.Bool                       // 停机中（结果不写终态，改为放租约）
	inflightTasks map[*asynctask.AsyncTask]struct{} // 在途任务，用于 Shutdown 放租约
	exited        chan struct{}                     // 所有 goroutine 退出后关闭

	// 类型级启停开关的本地缓存（TypeGateTTL 内有效）。
	gateMu    sync.Mutex
	gateCache map[string]gateCacheEntry
}

// WorkerConfig 执行运行时配置。
type WorkerConfig struct {
	Concurrency     int           // 每类型执行槽数，默认 10
	ShutdownTimeout time.Duration // 默认 15s
	// WorkerID 是执行者标识，用于事件/存活上报；任务租约的归属标识由 Source 决定。
	WorkerID     string
	Logger       asynctask.Logger // 默认 asynctask.DefaultLogger()
	EventHandler func(Event)      // 事件回调

	// LeaseTTL 是任务租约时长，默认 60s；执行期按 1/3 周期续租。
	//
	// 必须与 Source 侧的租约时长一致（manager.Config.LeaseTTL / db.Config.LeaseTTL）：
	// 小了会导致正常执行被误判失权，大了会让崩溃后的接管变慢。
	LeaseTTL time.Duration

	// PollInterval 是「无唤醒信号」时的轮询间隔，默认 500ms。
	PollInterval time.Duration

	// FallbackPollInterval 是「有唤醒信号」时的兜底轮询周期（默认 30s）。
	// 通知器只负责尽快叫醒执行侧，不参与正确性：信号丢了由这个兜底定时器补上。
	FallbackPollInterval time.Duration

	// WakeSource 是唤醒信号来源（可选）。非 nil 且对该类型返回非 nil 通道时，
	// 等待改为「定时器或唤醒信号，谁先到就 Fetch 一次」，定时器取 FallbackPollInterval。
	WakeSource WakeSource

	// HealthReporter 是可选存活上报器：非 nil 且 HealthReportInterval>0 时起 ticker 周期上报。
	HealthReporter HealthReporter
	// HealthReportInterval 存活上报周期；<=0 关闭（默认 0）。
	HealthReportInterval time.Duration

	// TypeGate 类型级启停开关（可选）。nil 时视为全启用（行为不变）。
	TypeGate TypeGate
	// TypeGateTTL worker 侧缓存开关状态的 TTL，默认 5s；<=0 每次查询（不缓存）。
	TypeGateTTL time.Duration
}

// TypeGate 是类型级启停开关接口（可选）。
// 实现返回 (false, nil) 表示该类型被禁用、不应认领；返回错误时 worker 按 fail-open 视为启用。
// 依赖边界：worker 只依赖本接口，具体实现（查库+缓存）由接入方提供，不进 worker 包。
type TypeGate interface {
	TypeEnabled(ctx context.Context, taskType string) (bool, error)
}

// HealthReporter 是 worker 主动存活上报接口（可选）。
// 依赖边界：worker 只依赖本接口，具体实现（DB/Redis/HTTP）由接入方提供，不进 worker 包。
type HealthReporter interface {
	ReportAlive(ctx context.Context, workerID string, running int) error
}

// WakeSource 抽象「某任务类型有新任务」的唤醒信号来源。
//
// 实现返回一个每类型可读的通道；每次收到（读出一个值）即代表「该类型有新任务，
// 去 Fetch 一次」。通道关闭或 ctx 取消表示不再有信号。Worker 侧读取即消费。
type WakeSource interface {
	// Wake 返回该任务类型的唤醒通道；返回 nil 表示该类型不可用唤醒（退回纯轮询）。
	Wake(taskType string) <-chan struct{}
}

// renewMaxConsecutiveFailures 是「续租连续失败多少次判定失权」的阈值。
//
// 单次失败可能只是数据库瞬时抖动，连续失败才按失权处理（与 remote 侧约定一致）。
const renewMaxConsecutiveFailures = 2

// handlerEntry 是某任务类型的注册项：处理器 + 选项 + 该类型的默认值。
type handlerEntry struct {
	handler Handler
	opts    handlerOptions
}

// New 构造执行运行时。src 为 nil 时返回一个「毒化」Worker：Register/Start 会返回 ErrInvalidConfig。
func New(src asynctask.Source, cfg *WorkerConfig) *Worker {
	if src == nil {
		return &Worker{src: nil, logger: asynctask.DefaultLogger()}
	}
	var c WorkerConfig
	if cfg != nil {
		c = *cfg
	}
	applyDefaults(&c)
	logger := c.Logger
	if logger == nil {
		logger = asynctask.DefaultLogger()
	}
	c.Logger = logger
	return &Worker{
		src:      src,
		cfg:      c,
		logger:   logger,
		handlers: make(map[string]*handlerEntry),
	}
}

func applyDefaults(c *WorkerConfig) {
	if c.Concurrency <= 0 {
		c.Concurrency = 10
	}
	if c.LeaseTTL <= 0 {
		c.LeaseTTL = 60 * time.Second
	}
	if c.ShutdownTimeout <= 0 {
		c.ShutdownTimeout = 15 * time.Second
	}
	if c.PollInterval <= 0 {
		c.PollInterval = 500 * time.Millisecond
	}
	if c.FallbackPollInterval <= 0 {
		c.FallbackPollInterval = 30 * time.Second
	}
	if strings.TrimSpace(c.WorkerID) == "" {
		c.WorkerID = defaultWorkerID()
	}
	if c.TypeGate != nil && c.TypeGateTTL == 0 {
		c.TypeGateTTL = 5 * time.Second
	}
}

// gateCacheEntry 是类型开关的缓存项。
type gateCacheEntry struct {
	enabled   bool
	expiresAt time.Time
}

func defaultWorkerID() string {
	host, err := os.Hostname()
	if err != nil || host == "" {
		host = "unknown-host"
	}
	return fmt.Sprintf("%s-%d", host, os.Getpid())
}

// Register 注册任务类型与处理器，必须在 Run 之前。重复类型返回 ErrDuplicateType，
// Run 之后返回 ErrWorkerAlreadyRunning。
// RegisterResult 注册「带结果」的执行契约（推荐用于每类型并发 >1 的场景）。
//
// 与 Register 的唯一差别是 Handler 额外实现 ResultHandler：Execute 成功后 worker 从
// ExecuteResult 的返回值取结果快照写进 core_async_task.result。
func (w *Worker) RegisterResult(taskType string, h ResultHandler, opts ...HandlerOption) error {
	if h == nil {
		return asynctask.ErrNilHandler
	}
	return w.Register(taskType, resultHandlerAdapter{ResultHandler: h}, opts...)
}

func (w *Worker) Register(taskType string, h Handler, opts ...HandlerOption) error {
	if w.src == nil {
		return asynctask.ErrInvalidConfig
	}
	if strings.TrimSpace(taskType) == "" {
		return asynctask.ErrEmptyType
	}
	if h == nil {
		return asynctask.ErrNilHandler
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.running {
		return asynctask.ErrWorkerAlreadyRunning
	}
	if w.stopped {
		return asynctask.ErrWorkerStopped
	}
	if w.handlers == nil {
		w.handlers = make(map[string]*handlerEntry)
	}
	if _, ok := w.handlers[taskType]; ok {
		return asynctask.ErrDuplicateType
	}

	var o handlerOptions
	for _, opt := range opts {
		if opt != nil {
			opt.applyHandler(&o)
		}
	}

	w.handlers[taskType] = &handlerEntry{handler: h, opts: o}
	w.order = append(w.order, taskType)
	return nil
}

// Start 异步启动运行：完成校验、建立运行期上下文并起 fetcher/执行槽后立即返回，不阻塞。
//
// 启动期错误经返回值上报：未注册类型的 ErrNoType、已停止的 ErrWorkerStopped、
// 已在运行的 ErrWorkerAlreadyRunning、src 为 nil 的 ErrInvalidConfig。
func (w *Worker) Start(ctx context.Context) error {
	if w.src == nil {
		return asynctask.ErrInvalidConfig
	}
	if ctx == nil {
		ctx = context.Background()
	}

	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return asynctask.ErrWorkerStopped
	}
	if w.running {
		w.mu.Unlock()
		return asynctask.ErrWorkerAlreadyRunning
	}
	if len(w.handlers) == 0 {
		w.mu.Unlock()
		return asynctask.ErrNoType
	}
	w.running = true
	w.startedAt = time.Now()
	// baseCtx 是执行期上下文：随调用方 ctx 取消，但不随 Shutdown 停 Fetch 而取消，
	// 以便在 Shutdown 的「等在途」阶段任务仍能正常收尾。
	baseCtx, baseCancel := context.WithCancel(ctx)
	// fetchCtx 专用于 Fetch/等待与认领循环；Shutdown 先取消它来停 Fetch。
	fetchCtx, fetchCancel := context.WithCancel(baseCtx)
	w.baseCtx = baseCtx
	w.baseCancel = baseCancel
	w.fetchCtx = fetchCtx
	w.fetchCancel = fetchCancel
	w.exited = make(chan struct{})
	exited := w.exited
	types := append([]string(nil), w.order...)
	w.mu.Unlock()

	var wg sync.WaitGroup
	for _, taskType := range types {
		n := w.concurrencyFor(taskType)
		// ready 里放的是「空闲执行槽的任务通道」，容量 = 槽数，启动即填满。
		ready := make(chan chan *asynctask.AsyncTask, n)
		for i := 0; i < n; i++ {
			slotCh := make(chan *asynctask.AsyncTask, 1)
			ready <- slotCh
			wg.Add(1)
			go func(tt string, ch chan *asynctask.AsyncTask) {
				defer wg.Done()
				w.slotLoop(tt, ch, ready)
			}(taskType, slotCh)
		}
		wg.Add(1)
		go func(tt string, rd chan chan *asynctask.AsyncTask) {
			defer wg.Done()
			w.fetchLoop(tt, rd)
		}(taskType, ready)
	}

	// 内部 goroutine 负责「等所有槽与 fetcher 退出 → 关 exited」，Start 本身立即返回。
	go func() {
		wg.Wait()
		close(exited)
	}()

	// 可选存活上报：起 ticker 周期回调 HealthReporter（仅当 reporter 非 nil 且 interval>0）。
	if w.cfg.HealthReporter != nil && w.cfg.HealthReportInterval > 0 {
		go w.healthReportLoop(fetchCtx)
	}

	return nil
}

// healthReportLoop 周期上报存活：间隔 HealthReportInterval，running=当前在跑任务数。
func (w *Worker) healthReportLoop(ctx context.Context) {
	ticker := time.NewTicker(w.cfg.HealthReportInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.mu.Lock()
			running := len(w.inflightTasks)
			w.mu.Unlock()
			if err := w.cfg.HealthReporter.ReportAlive(context.WithoutCancel(ctx), w.cfg.WorkerID, running); err != nil {
				w.logger.Warnw(ctx, "asynctask health report failed", "worker_id", w.cfg.WorkerID, "error", err)
			}
		}
	}
}

// Run 阻塞运行，直到 ctx 取消或 Shutdown。未注册类型返回 ErrNoType，重复 Run 返回 ErrWorkerAlreadyRunning。
func (w *Worker) Run(ctx context.Context) error {
	if err := w.Start(ctx); err != nil {
		return err
	}

	w.mu.Lock()
	exited := w.exited
	baseCtx := w.baseCtx
	w.mu.Unlock()

	if exited != nil {
		<-exited
	}
	if baseCtx != nil {
		if err := baseCtx.Err(); err != nil {
			return err
		}
	}
	return nil
}

// Shutdown 优雅停机：停认领 → 等在途 → 放租约（不是续租）。未 Run 时返回 nil（幂等）。
//
// 停机导致的在途取消不会写成终态 canceled：那会吞掉剩余重试次数；
// 改为调用 Source.Release 把租约置为已过期，让其它实例（或本进程重启后）重跑。
func (w *Worker) Shutdown(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	w.mu.Lock()
	if w.stopped {
		w.mu.Unlock()
		return nil
	}
	if !w.running {
		// 未 Run：幂等返回 nil，但标记 stopped，使后续 Run 返回 ErrWorkerStopped。
		w.stopped = true
		w.mu.Unlock()
		return nil
	}
	w.stopped = true
	baseCancel := w.baseCancel
	fetchCancel := w.fetchCancel
	exited := w.exited
	w.mu.Unlock()

	// 1. 先停认领：取消取任务 ctx，让 fetcher 与空闲槽退出。
	w.fetchStop.Store(true)
	if fetchCancel != nil {
		fetchCancel()
	}

	// 2. 等在途任务收尾，最多 ShutdownTimeout（或调用方 ctx 更早结束）。
	timer := time.NewTimer(w.cfg.ShutdownTimeout)
	defer timer.Stop()

	done := make(chan struct{})
	go func() {
		if exited != nil {
			<-exited
		}
		close(done)
	}()

	select {
	case <-done:
	case <-timer.C:
	case <-ctx.Done():
	}

	// 3. 对仍未收尾的任务放租约（Release 把 lease_until 置为现在），让接管方尽快接手。
	w.releaseInflight()

	// 4. 收尾：释放执行上下文（未收尾任务的 goroutine 会感知取消而结束）。
	if baseCancel != nil {
		baseCancel()
	}

	return nil
}

// releaseInflight 对仍标记为在途的任务调用 src.Release 放弃租约。
func (w *Worker) releaseInflight() {
	w.mu.Lock()
	tasks := make([]*asynctask.AsyncTask, 0, len(w.inflightTasks))
	for asyncTask := range w.inflightTasks {
		tasks = append(tasks, asyncTask)
	}
	w.mu.Unlock()

	for _, t := range tasks {
		w.releaseTask(t)
	}
}

// releaseTask 放弃单条任务的租约；失败只记日志（接手方最晚等租约过期也会拿到）。
func (w *Worker) releaseTask(t *asynctask.AsyncTask) {
	if t == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := w.src.Release(ctx, t); err != nil {
		w.logger.Warnw(ctx, "asynctask worker release lease failed",
			"task_type", t.Type, "async_task_id", t.ID, "error", err)
	}
}

// concurrencyFor 返回某任务类型的执行槽数：取 WithHandlerConcurrency，未设置回落全局 Concurrency。
func (w *Worker) concurrencyFor(taskType string) int {
	if e := w.handlers[taskType]; e != nil && e.opts.concurrency > 0 {
		return e.opts.concurrency
	}
	return w.cfg.Concurrency
}

// fetchLoop 是某任务类型唯一的认领者：拿到空闲执行槽 → 认领 → 交付给该槽。
//
// 拿不到空闲槽就阻塞在 ready 上（背压），因此队列积压时不会「认领了却没人执行」，
// 空闲时该类型也只有一个 goroutine 在查库（不会随并发数放大轮询）。
func (w *Worker) fetchLoop(taskType string, ready chan chan *asynctask.AsyncTask) {
	for {
		if w.fetchCtx.Err() != nil {
			return
		}

		// 1. 占一个空闲执行槽。
		var slot chan *asynctask.AsyncTask
		select {
		case slot = <-ready:
		case <-w.fetchCtx.Done():
			return
		}

		// 2. 类型级启停：禁用则不认领（任务留 pending、不丢），等待后重查。
		if !w.typeEnabled(taskType) {
			if !w.returnSlot(ready, slot) {
				return
			}
			if !w.wait(taskType) {
				return
			}
			continue
		}

		asyncTask, err := w.src.Fetch(w.fetchCtx, taskType)
		switch {
		case errors.Is(err, asynctask.ErrNoAsyncTask) || (err == nil && asyncTask == nil):
			if !w.returnSlot(ready, slot) {
				return
			}
			if !w.wait(taskType) {
				return
			}
			continue
		case err != nil:
			if !w.returnSlot(ready, slot) {
				return
			}
			// Fetch 的其它错误（含 ctx 取消）：记日志后等待重试/退出。
			if w.fetchCtx.Err() != nil {
				return
			}
			w.logger.Errorw(w.fetchCtx, "asynctask worker fetch error", "task_type", taskType, "error", err)
			if !w.wait(taskType) {
				return
			}
			continue
		}

		// 3. 交付给持有的槽（缓冲 1，必不阻塞）。停机则放租约退出。
		if w.fetchCtx.Err() != nil {
			w.releaseTask(asyncTask)
			return
		}
		select {
		case slot <- asyncTask:
		case <-w.fetchCtx.Done():
			w.releaseTask(asyncTask)
			return
		}
	}
}

// returnSlot 归还执行槽；停机返回 false。
func (w *Worker) returnSlot(ready chan chan *asynctask.AsyncTask, slot chan *asynctask.AsyncTask) bool {
	select {
	case ready <- slot:
		return true
	case <-w.fetchCtx.Done():
		return false
	}
}

// slotLoop 是一个执行槽：阻塞等待任务 → 执行 → 归还执行槽。
func (w *Worker) slotLoop(taskType string, jobs chan *asynctask.AsyncTask, ready chan chan *asynctask.AsyncTask) {
	defer func() {
		// 停机时把「已交付但还没开始执行」的任务放租约，避免它白等到租约过期。
		select {
		case asyncTask := <-jobs:
			if asyncTask != nil {
				w.releaseTask(asyncTask)
			}
		default:
		}
	}()
	for {
		select {
		case <-w.fetchCtx.Done():
			return
		case asyncTask := <-jobs:
			if asyncTask == nil {
				continue
			}
			w.trackInflight(asyncTask, true)
			w.process(taskType, asyncTask)
			w.trackInflight(asyncTask, false)
			if !w.returnSlot(ready, jobs) {
				return
			}
		}
	}
}

// typeEnabled 判断某类型是否启用：无 TypeGate 视为启用；gate 返回错误时 fail-open 视为启用。
func (w *Worker) typeEnabled(taskType string) bool {
	if w.cfg.TypeGate == nil {
		return true
	}
	if w.cfg.TypeGateTTL > 0 {
		w.gateMu.Lock()
		if e, ok := w.gateCache[taskType]; ok && time.Now().Before(e.expiresAt) {
			w.gateMu.Unlock()
			return e.enabled
		}
		w.gateMu.Unlock()
	}
	enabled, err := w.cfg.TypeGate.TypeEnabled(w.fetchCtx, taskType)
	if err != nil {
		return true // fail-open
	}
	if w.cfg.TypeGateTTL > 0 {
		w.gateMu.Lock()
		if w.gateCache == nil {
			w.gateCache = make(map[string]gateCacheEntry)
		}
		w.gateCache[taskType] = gateCacheEntry{enabled: enabled, expiresAt: time.Now().Add(w.cfg.TypeGateTTL)}
		w.gateMu.Unlock()
	}
	return enabled
}

// wait 等待「定时器 或 唤醒信号」，谁先到就返回 true（可继续，去 Fetch 一次）；停机返回 false。
//
// 定时器：接入唤醒信号（WakeSource 对该类型返回非 nil 通道）时取
// FallbackPollInterval（兜底轮询，默认 30s），否则取 PollInterval（默认 500ms）。
func (w *Worker) wait(taskType string) bool {
	interval := w.cfg.PollInterval
	var wake <-chan struct{}
	if w.cfg.WakeSource != nil {
		if ch := w.cfg.WakeSource.Wake(taskType); ch != nil {
			wake = ch
			interval = w.cfg.FallbackPollInterval
		}
	}

	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-w.fetchCtx.Done():
		return false
	case <-wake:
		return true
	case <-timer.C:
		return true
	}
}

// trackInflight 记录/移除在途任务（用于 Shutdown 放租约），由 mu 保护。
func (w *Worker) trackInflight(asyncTask *asynctask.AsyncTask, add bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.inflightTasks == nil {
		w.inflightTasks = make(map[*asynctask.AsyncTask]struct{})
	}
	if add {
		w.inflightTasks[asyncTask] = struct{}{}
	} else {
		delete(w.inflightTasks, asyncTask)
	}
}

// HandlerOption 是 Register 的每类型选项。封闭接口（applyHandler 未导出）。
type HandlerOption interface{ applyHandler(*handlerOptions) }

// handlerOptions 是每类型选项载体。
type handlerOptions struct {
	timeout time.Duration
	// concurrency 该任务类型的执行槽数；0 表示未设置，回落到 WorkerConfig.Concurrency。
	concurrency int
}

type handlerOptionFunc func(*handlerOptions)

func (f handlerOptionFunc) applyHandler(o *handlerOptions) { f(o) }

// WithHandlerTimeout 设置该类型单次执行超时（覆盖 Config 默认）。
func WithHandlerTimeout(d time.Duration) HandlerOption {
	return handlerOptionFunc(func(o *handlerOptions) { o.timeout = d })
}

// WithHandlerConcurrency 设置该任务类型的执行槽数（独立并发，覆盖 WorkerConfig.Concurrency）。
// n<=0 视为未设置，回落到 WorkerConfig.Concurrency。
func WithHandlerConcurrency(n int) HandlerOption {
	return handlerOptionFunc(func(o *handlerOptions) {
		if n > 0 {
			o.concurrency = n
		}
	})
}

// Event 是执行过程事件，通过 WorkerConfig.EventHandler 回调（仅用于观察，不参与正确性）。
type Event struct {
	Type        string // 事件类型（如 started / succeeded / failed / retried / canceled）
	AsyncTaskID string
	TaskType    string
	WorkerID    string
	Attempt     int
	Duration    time.Duration
	Err         error
}
