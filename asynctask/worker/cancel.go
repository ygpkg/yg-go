package worker

// 取消通知（设计 §9.1 / Y1–Y2）：执行侧维护一张「本地活跃执行表」，
// 常驻监听器收到 Cancel 事务内发出的 PG 通知后，按 (任务 id, generation, attempt)
// 精确匹配并取消那次执行。
//
// 三条保障叠在一起才是完整的取消语义：
//  1. 通知：快（亚秒级），但可能丢、可能重连期间到不了；
//  2. 登记后立刻复查 DB：补上「取消事务先提交、执行后登记」这个竞态窗口；
//  3. 定时补查：监听断线/丢通知的兜底（续租失权仍然是最外层兜底）。
//
// 本文件只做「本地活跃执行表 + 通知消费 + 补查」，不碰取单唤醒：
// 唤醒（WakeSource / Notifier）回答「有没有新任务」，取消通知回答「这次执行要停」，
// 两者的消费方、去重口径、失败处理都不同，混成一个通道会让「漏一条唤醒」变成「漏一次取消」。

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// cancelWatchRetryInterval 是「监听通道关闭/启动失败」后的重连间隔。
const cancelWatchRetryInterval = 3 * time.Second

// cancelKey 是本地活跃执行表的键：任务 id + generation + attempt 三元组唯一定位一次 run。
type cancelKey struct {
	taskID     string
	generation int
	attempt    int
}

// cancelEntry 是一次在途执行的可取消句柄。
type cancelEntry struct {
	cancel context.CancelFunc
	// task 是可读的任务快照：补查时用来定位任务；取消命中时用来打日志。
	task *asynctask.AsyncTask
}

// cancelRegistry 是「本地活跃执行表」：本进程当前在跑的执行 → 取消函数。
//
// 为什么键里必须带 generation+attempt：取消通知可能晚到（网络/重连/积压），
// 此时任务可能已经被重放（generation+1）或重投（attempt+1）。
// 只按任务 id 匹配会把「晚到的旧取消」用到新执行上——这正是要避免的误杀。
type cancelRegistry struct {
	mu sync.Mutex
	m  map[cancelKey]cancelEntry
}

func newCancelRegistry() *cancelRegistry {
	return &cancelRegistry{m: make(map[cancelKey]cancelEntry)}
}

// register 登记一次在途执行；同键重复登记（理论上不会发生）时覆盖。
func (r *cancelRegistry) register(key cancelKey, task *asynctask.AsyncTask, cancel context.CancelFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.m[key] = cancelEntry{cancel: cancel, task: task}
}

// unregister 注销一次在途执行。
func (r *cancelRegistry) unregister(key cancelKey) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.m, key)
}

// cancelMatching 按通知精确匹配并取消；返回命中的执行数。
//
// 0 命中是正常情况（通知晚到、或任务还在 pending 没有在途执行），不是错误。
func (r *cancelRegistry) cancelMatching(sig asynctask.CancelSignal) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	hit := 0
	for key, entry := range r.m {
		if !sig.Match(key.taskID, key.generation, key.attempt) {
			continue
		}
		entry.cancel()
		hit++
	}
	return hit
}

// snapshot 返回当前在途执行的键与任务快照（补查用）。
func (r *cancelRegistry) snapshot() []struct {
	Key  cancelKey
	Task *asynctask.AsyncTask
} {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]struct {
		Key  cancelKey
		Task *asynctask.AsyncTask
	}, 0, len(r.m))
	for key, entry := range r.m {
		out = append(out, struct {
			Key  cancelKey
			Task *asynctask.AsyncTask
		}{Key: key, Task: entry.task})
	}
	return out
}

// len 返回当前在途执行数（观测/测试用）。
func (r *cancelRegistry) len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.m)
}

// cancelWatchLoop 常驻消费取消通知；通道关闭或 Watch 失败时按固定间隔重连。
//
// 循环本身不判断「该不该取消」：那是 cancelMatching 按 (id,generation,attempt) 做的，
// 这里只负责「把通知变成一次本地查找」。
func (w *Worker) cancelWatchLoop(ctx context.Context) {
	for {
		if ctx.Err() != nil {
			return
		}
		ch, err := w.cfg.CancelWatcher.Watch(ctx)
		if err != nil {
			// Watch 首次调用会同步建连（如 cancelwatch 的 PG LISTEN），停机正好落在建连途中时
			// 这里拿到的是 context.Canceled——那是正常退出，不是需要重试的故障。
			if ctx.Err() != nil {
				return
			}
			w.logger.Warnw(ctx, "asynctask cancel watch failed, retrying",
				"worker_id", w.cfg.WorkerID, "retry_after", cancelWatchRetryInterval, "error", err)
			if !sleepCtx(ctx, cancelWatchRetryInterval) {
				return
			}
			continue
		}
		if ch == nil {
			// 实现不给通道：不要空转，退化成定时补查（若有）。
			w.logger.Warnw(ctx, "asynctask cancel watcher returned nil channel",
				"worker_id", w.cfg.WorkerID)
			return
		}
		if !w.consumeCancel(ctx, ch) {
			return
		}
		// 通道关闭：等一会儿再重连（避免实现立刻失败时忙等）。
		if !sleepCtx(ctx, cancelWatchRetryInterval) {
			return
		}
	}
}

// consumeCancel 消费一个通道直到它关闭；返回 false 表示 ctx 结束、应直接退出。
func (w *Worker) consumeCancel(ctx context.Context, ch <-chan asynctask.CancelSignal) bool {
	for {
		select {
		case <-ctx.Done():
			return false
		case sig, ok := <-ch:
			if !ok {
				return true
			}
			hit := w.cancelReg.cancelMatching(sig)
			w.logger.Infow(ctx, "asynctask cancel signal received",
				"worker_id", w.cfg.WorkerID, "async_task_id", sig.TaskID,
				"generation", sig.Generation, "attempt", sig.Attempt, "matched", hit)
		}
	}
}

// cancelRecheckLoop 定时补查在途执行的数据库状态：监听断线、通知丢失、实现故障时兜底。
//
// 补查代价是「在途执行数 × 一次主键查询」，默认周期 30s（远小于历史上「等一个续租周期」
// 的问题，但也不需要像通知那样快）。
func (w *Worker) cancelRecheckLoop(ctx context.Context) {
	interval := w.cfg.CancelRecheckInterval
	if interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.recheckActive(ctx)
		}
	}
}

// recheckActive 对每个在途执行查一次状态，已不在 running 的一律取消。
//
// 判定口径：只有 StatusRunning 视为「继续跑」；终态（canceled/failed/...）与
// 「任务已不存在（被清理）」都取消——继续跑的结果也写不回去，白烧算力。
func (w *Worker) recheckActive(ctx context.Context) {
	reader, ok := w.src.(asynctask.StatusReader)
	if !ok {
		return
	}
	for _, item := range w.cancelReg.snapshot() {
		if item.Task == nil {
			continue
		}
		status, err := reader.TaskStatus(ctx, item.Task.ID)
		if err != nil {
			if errors.Is(err, asynctask.ErrAsyncTaskNotFound) {
				w.cancelByKey(item.Key, "task_gone")
				continue
			}
			w.logger.Warnw(ctx, "asynctask cancel recheck failed",
				"worker_id", w.cfg.WorkerID, "async_task_id", item.Task.ID, "error", err)
			continue
		}
		if status != asynctask.StatusRunning {
			w.cancelByKey(item.Key, string(status))
		}
	}
}

// recheckOnRegister 在登记后立刻复查一次任务状态，消掉「取消事务先提交、执行后登记」的竞态。
//
// 顺序很重要：先登记再复查。反过来（先查后登）会留下「查完到登记之间被取消」的窗口，
// 而那个窗口里的通知恰好落不到任何在途执行上。
func (w *Worker) recheckOnRegister(ctx context.Context, task *asynctask.AsyncTask) {
	reader, ok := w.src.(asynctask.StatusReader)
	if !ok || task == nil {
		return
	}
	status, err := reader.TaskStatus(ctx, task.ID)
	if err != nil {
		// 查不到就按「继续跑」处理：租约仍在手上，真取消了下一次续租/补查会发现。
		return
	}
	if status != asynctask.StatusRunning {
		w.cancelByKey(cancelKey{taskID: task.ID, generation: task.Generation, attempt: task.Attempt},
			"register_recheck_"+string(status))
	}
}

// cancelByKey 按本地活跃执行表的键取消一次执行。
func (w *Worker) cancelByKey(key cancelKey, reason string) {
	hit := w.cancelReg.cancelMatching(asynctask.CancelSignal{
		TaskID:     key.taskID,
		Generation: key.generation,
		Attempt:    key.attempt,
	})
	w.logger.Infow(context.Background(), "asynctask execution canceled",
		"worker_id", w.cfg.WorkerID, "async_task_id", key.taskID,
		"generation", key.generation, "attempt", key.attempt, "reason", reason, "matched", hit)
}

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

// registerActive 登记一次在途执行（cancelReg 未初始化时静默跳过：未 Start 的场景没有取消语义）。
func (w *Worker) registerActive(key cancelKey, task *asynctask.AsyncTask, cancel context.CancelFunc) {
	if w.cancelReg == nil {
		return
	}
	w.cancelReg.register(key, task, cancel)
}

// unregisterActive 注销一次在途执行。
func (w *Worker) unregisterActive(key cancelKey) {
	if w.cancelReg == nil {
		return
	}
	w.cancelReg.unregister(key)
}
