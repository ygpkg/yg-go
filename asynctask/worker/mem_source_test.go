package worker_test

import (
	"context"
	"sync"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
)

// memSource 是内存队列版 Source 测试替身，供 W1 自测 Worker 而不依赖 manager。
//
// 行为：
//   - Fetch：有任务则出队；无任务返回 asynctask.ErrNoAsyncTask。
//   - Save：记录本次 Result 与所选回调是否被调用；并模拟 Source 的重试决策——
//     可重试的失败会把任务以 Attempt+1 重新入队（供「失败重试」用例观察多次尝试）。
//   - Renew：记录调用次数（供停机放租约用例断言）。
type memSource struct {
	mu     sync.Mutex
	queues map[string][]*asynctask.AsyncTask

	results   []recordedResult       // 每次 Save 的客观结果（含结果快照）
	renews    []*asynctask.AsyncTask // 每次 Renew 的任务（执行期续租证据）
	releases  []*asynctask.AsyncTask // 每次 Release 的任务（停机/交付失败放租约证据）
	callbacks int                    // 被实际调用的钩子次数
	hooks     []string               // 被调用的钩子名（on_success / on_retry / on_failure / on_canceled）

	// retryUpperBound 控制「可重试的失败」最多重新入队几次，避免无限循环。
	retryUpperBound int

	// fetchCount 认领次数（用于断言「每类型单点轮询」不随并发放大）。
	fetchCount int
	// renewErr 非 nil 时 Renew 直接返回它（模拟失权/故障）。
	renewErr error
}

var _ asynctask.Source = (*memSource)(nil)

type recordedResult struct {
	AsyncTaskID string
	Outcome     asynctask.Outcome
	Failure     asynctask.FailureKind
	Err         error
	// Payload 是本次 Save 收到的结果快照（ResultProvider 用例断言用）。
	Payload asynctask.Result
}

func newMemSource() *memSource {
	return &memSource{
		queues:          make(map[string][]*asynctask.AsyncTask),
		retryUpperBound: 3,
	}
}

// push 把任务放入指定类型的队列。
func (m *memSource) push(taskType string, task *asynctask.AsyncTask) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if task.Type == "" {
		task.Type = taskType
	}
	m.queues[taskType] = append(m.queues[taskType], task)
}

// Fetch 从队列取一条；无任务返回 ErrNoAsyncTask。
func (m *memSource) Fetch(ctx context.Context, taskType string) (*asynctask.AsyncTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.fetchCount++
	q := m.queues[taskType]
	if len(q) == 0 {
		return nil, asynctask.ErrNoAsyncTask
	}
	task := q[0]
	m.queues[taskType] = q[1:]
	return task, nil
}

// Renew 记录续租调用（内存实现无实际租约）。
func (m *memSource) Renew(ctx context.Context, task *asynctask.AsyncTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.renews = append(m.renews, task)
	return m.renewErr
}

// Release 记录放租约调用（停机或认领后无法交付时）。
func (m *memSource) Release(ctx context.Context, task *asynctask.AsyncTask) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.releases = append(m.releases, task)
	return nil
}

// Save 记录结果与回调调用，并按「本地 Source」的语义模拟重试流转。
func (m *memSource) Save(ctx context.Context, task *asynctask.AsyncTask, r asynctask.Result, callback asynctask.EnsureFunc) error {
	m.mu.Lock()
	m.results = append(m.results, recordedResult{
		AsyncTaskID: task.ID,
		Outcome:     r.Outcome,
		Failure:     r.Failure,
		Err:         r.Err,
		Payload:     r,
	})
	// 模拟 Source 的重试决策：仅当 outcome=failed 且仍有失败额度时把任务重新入队。
	// 重试同时让 attempt+1（下一次认领会推进执行序号）与 failure_count+1（本次失败计费）。
	requeue := false
	if r.Outcome == asynctask.OutcomeFailed && m.retryUpperBound > 0 && task.HasRemainingAttempts() {
		m.retryUpperBound--
		task.Attempt++
		task.FailureCount++
		requeue = true
	}
	m.mu.Unlock()

	// Source 在事务内调用 worker 交来的回调（本地形态下）。
	if callback != nil {
		m.recordHookCallback(r.Outcome)
		_ = callback(ctx)
	}

	if requeue {
		m.mu.Lock()
		m.queues[task.Type] = append(m.queues[task.Type], task)
		m.mu.Unlock()
	}
	return nil
}

// recordHookCallback 根据 outcome 归类被调用的钩子（worker 只传一个回调，故此处按 outcome 记录）。
func (m *memSource) recordHookCallback(outcome asynctask.Outcome) {
	name := string(outcome)
	switch outcome {
	case asynctask.OutcomeSucceeded:
		name = "on_success"
	case asynctask.OutcomeCanceled:
		name = "on_canceled"
	case asynctask.OutcomeFailed:
		name = "on_failure_or_retry" // worker 决定的那个回调；具体由 handler 侧区分
	case asynctask.OutcomePaused:
		name = "on_pause"
	}
	m.mu.Lock()
	m.callbacks++
	m.hooks = append(m.hooks, name)
	m.mu.Unlock()
}

// snapshot 返回结果的拷贝，供断言。
func (m *memSource) snapshot() ([]recordedResult, int, []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := append([]recordedResult(nil), m.results...)
	hooks := append([]string(nil), m.hooks...)
	return res, m.callbacks, hooks
}

// fetches 返回 Fetch 调用次数。
func (m *memSource) fetches() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.fetchCount
}

// releaseCount 返回 Release 调用次数。
func (m *memSource) releaseCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.releases)
}

// renewCount 返回 Renew 调用次数。
func (m *memSource) renewCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.renews)
}

// Close 空操作。
func (m *memSource) Close(ctx context.Context) error { return nil }

// ensure time is referenced even if unused in some build tags.
var _ = time.Now
