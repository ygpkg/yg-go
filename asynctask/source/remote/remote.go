// Package remote 实现 asynctask.Source 的 HTTP 版本：向一个独立部署的「远程任务中心」
// 取任务（Fetch）、续租（Renew）、放租约（Release）、回传结果（Save）。
//
// 依赖约束（A10）：本包只依赖根包 asynctask，不引入 gorm / asynq /
// asynctask/manager / asynctask/worker。远程形态下事务性不存在——
// asynctask.TxFromContext 在本包调用钩子时始终返回 nil，业务钩子不得假设能拿到 tx。
//
// 端点约定（与契约文档「Source（取任务接口）」及「远程 Source（source/remote）」一致）：
//
//	Fetch   → POST {base}/GetPendingAsyncTask { task_type, worker_id }
//	Renew   → POST {base}/Heartbeat      { task_id, worker_id }
//	Release → POST {base}/Release        { task_id, worker_id }（中心未实现 404/405 时按无操作处理）
//	Save    → 调本地钩子 → POST {base}/Callback { task_id, worker_id, status, result, error }
//	Close   → 关闭 HTTP 连接
package remote

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/wire"
)

// 默认参数。Config 中未显式给出的字段按这些值解析。
const (
	DefaultTimeout            = 10 * time.Second
	DefaultCallbackMaxRetry   = 3
	DefaultCallbackRetryDelay = 200 * time.Millisecond
	// FetchNoAsyncTaskBackoffInitial / FetchNoAsyncTaskBackoffMax 是「Fetch 返回 ErrNoAsyncTask 时
	// Worker 侧退避」的默认区间（500ms 起，指数到 5s）。见 NoAsyncTaskBackoff 与 Fetch 的 godoc。
	FetchNoAsyncTaskBackoffInitial = 500 * time.Millisecond
	FetchNoAsyncTaskBackoffMax     = 5 * time.Second
)

// 回调重试间隔（3 次重试之间的等待）：200ms / 1s / 3s。
var defaultCallbackRetryDelays = []time.Duration{
	200 * time.Millisecond,
	1 * time.Second,
	3 * time.Second,
}

var (
	// ErrRenewLost 是 Renew 连续失败达到阈值（默认 2 次）时返回的哨兵错误。
	//
	// 等价于根包 asynctask.ErrLeaseLost（保留本名字仅为向后兼容）：
	// Renew 第 2 次「连续」失败即视为失权，Worker 侧据此取消本次执行的 ctx。
	// 任何一次成功都会把连续失败计数清零。
	ErrRenewLost = asynctask.ErrLeaseLost

	// ErrCallbackLost 是 Save 期间 Callback POST 在重试耗尽后仍失败时返回的哨兵错误。
	// 接入方应据此兜底（记日志 / 触发告警 / 人工核对），中心侧可能仍认为任务在跑。
	//
	// 注：根包 asynctask 未提供等价哨兵（其错误集合里没有 callback-lost 语义），
	// 故在 remote 包内自定义；前缀沿用包约定「asynctask: 」。
	ErrCallbackLost = errors.New("asynctask: remote callback lost")

	// ErrBadResponse 是远程返回了无法解析或不满足契约的响应体。
	ErrBadResponse = errors.New("asynctask: remote bad response")
)

// Config 是远程 Source 的配置。至少需要 BaseURL 与 WorkerID。
type Config struct {
	// BaseURL 是远程任务中心的根地址，如 "https://center.example.com/asynctask"。
	// 会与各端点拼接（BaseURL + "/GetPendingAsyncTask"）。
	BaseURL string
	// WorkerID 是本执行节点标识，随每个请求上报，用于中心侧归属与心跳判定。
	WorkerID string

	// HTTPClient 可选。为 nil 时本包自建一个带 Timeout 的 *http.Client，
	// 并在 Close 时对其调用 CloseIdleConnections。传入自定义 client 时由调用方负责生命周期。
	HTTPClient *http.Client
	// Timeout 是单次请求超时；仅在 HTTPClient 为 nil 时生效。默认 10s。
	Timeout time.Duration

	// CallbackMaxRetry 是 Callback 失败后的重试次数（不含首次），默认 3。
	CallbackMaxRetry int
	// CallbackRetryDelays 是每次重试前的等待，默认 [200ms, 1s, 3s]。
	// 长度不足时对超出部分使用最后一个值；为空时用默认序列。
	CallbackRetryDelays []time.Duration

	// RenewMaxFailures 是 Renew 连续失败多少次判定失权并返回 ErrRenewLost，默认 2。
	RenewMaxFailures int
}

// Source 是 asynctask.Source 的 HTTP 实现。并发安全（内部状态仅为 Renew 失败计数，
// 每次 Renew 对应一个任务，Worker 串行调用同一任务的 Renew）。
type Source struct {
	cfg    Config
	client *http.Client
	own    bool // client 是否由本包创建（决定 Close 是否关连接）

	// renewFails 是 Renew 连续失败计数。多个执行槽共用一个 Source，必须加锁。
	renewMu    sync.Mutex
	renewFails int
}

// 编译期断言：*Source 满足 asynctask.Source。
var _ asynctask.Source = (*Source)(nil)

// New 构造一个远程 Source。BaseURL 或 WorkerID 为空时返回 asynctask.ErrInvalidConfig 包装错误。
func New(cfg Config) (*Source, error) {
	if cfg.BaseURL == "" || cfg.WorkerID == "" {
		return nil, fmt.Errorf("%w: remote source requires BaseURL and WorkerID", asynctask.ErrInvalidConfig)
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultTimeout
	}
	if cfg.CallbackMaxRetry <= 0 {
		cfg.CallbackMaxRetry = DefaultCallbackMaxRetry
	}
	if len(cfg.CallbackRetryDelays) == 0 {
		cfg.CallbackRetryDelays = defaultCallbackRetryDelays
	}
	if cfg.RenewMaxFailures <= 0 {
		cfg.RenewMaxFailures = 2
	}

	s := &Source{cfg: cfg}
	if cfg.HTTPClient != nil {
		s.client = cfg.HTTPClient
	} else {
		s.client = &http.Client{Timeout: cfg.Timeout}
		s.own = true
	}
	return s, nil
}

// Fetch 取一条待执行任务：POST {base}/GetPendingAsyncTask。
//
//   - 中心返回「无任务」（available=false 或 HTTP 204）时返回 asynctask.ErrNoAsyncTask（不是错误，
//     也不是失权）。注意：本方法不 sleep、不内部重试，退避是 Worker 的责任——
//     拿 ErrNoAsyncTask 后由 Worker 按 NoAsyncTaskBackoff 计算等待时长（500ms 起、指数到 5s）。
//   - HTTP 5xx（或 429）视为「中心暂时不可用/过载」，返回一个非 ErrNoAsyncTask 的普通错误，
//     这样 Worker 会按错误链路退避而不是把 5xx 当「无任务」高频空转。
//   - 成功则解析出 *asynctask.AsyncTask，Attempt 由中心给出。
func (s *Source) Fetch(ctx context.Context, taskType string) (*asynctask.AsyncTask, error) {
	if taskType == "" {
		return nil, asynctask.ErrEmptyType
	}
	body, _ := json.Marshal(wire.FetchRequest{TaskType: taskType, WorkerID: s.cfg.WorkerID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.BaseURL+wire.PathFetch, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	// 无任务：204 或显式 available=false。
	if resp.StatusCode == http.StatusNoContent {
		return nil, asynctask.ErrNoAsyncTask
	}
	// 5xx / 429：不可用。绝不能返回 ErrNoAsyncTask（否则 Worker 会把它当正常空转并高频重试）。
	if resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("asynctask: remote GetPendingAsyncTask unavailable: status=%d body=%s", resp.StatusCode, bytes.TrimSpace(slurp))
	}
	if resp.StatusCode != http.StatusOK {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("%w: GetPendingAsyncTask status=%d body=%s", ErrBadResponse, resp.StatusCode, bytes.TrimSpace(slurp))
	}

	var out wire.FetchResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("%w: decode GetPendingAsyncTask: %v", ErrBadResponse, err)
	}
	if !out.Available || out.Task == nil {
		return nil, asynctask.ErrNoAsyncTask
	}
	return out.Task.ToAsyncTask(), nil
}

// Renew 向中心发心跳续租：POST {base}/Heartbeat。
//
// 失权语义：内部维护「连续失败」计数。任何一次成功都清零；连续失败达到
// RenewMaxFailures（默认 2）时，返回包装了 ErrRenewLost 的错误（errors.Is 可判定）。
// 由于 Source 不持有 Worker 的执行 ctx，无法在此直接 cancel——Worker 侧应识别
// ErrRenewLost 并取消本次执行 ctx，与本地形态「续租失败即失权」对齐。
func (s *Source) Renew(ctx context.Context, task *asynctask.AsyncTask) error {
	if task == nil || task.ID == "" {
		return asynctask.ErrAsyncTaskNotFound
	}
	body, _ := json.Marshal(wire.HeartbeatRequest{TaskID: task.ID, WorkerID: s.cfg.WorkerID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.BaseURL+wire.PathHeartbeat, bytes.NewReader(body))
	if err != nil {
		return s.renewFailed(err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.client.Do(req)
	if err != nil {
		return s.renewFailed(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return s.renewFailed(fmt.Errorf("Heartbeat status=%d body=%s", resp.StatusCode, bytes.TrimSpace(slurp)))
	}
	s.renewMu.Lock()
	s.renewFails = 0
	s.renewMu.Unlock()
	return nil
}

// renewFailed 累加连续失败计数；达到阈值时返回包装了 ErrRenewLost 的错误。
func (s *Source) renewFailed(cause error) error {
	s.renewMu.Lock()
	s.renewFails++
	fails := s.renewFails
	if fails >= s.cfg.RenewMaxFailures {
		// 达到阈值后重置计数，避免后续每次 Renew 都重复报 lost。
		s.renewFails = 0
	}
	s.renewMu.Unlock()
	if fails >= s.cfg.RenewMaxFailures {
		return fmt.Errorf("%w: %d consecutive heartbeat failures: %v", asynctask.ErrLeaseLost, s.cfg.RenewMaxFailures, cause)
	}
	return fmt.Errorf("asynctask: remote heartbeat failed (%d/%d): %w", fails, s.cfg.RenewMaxFailures, cause)
}

// Release 主动放弃所有权：POST {base}/Release。
//
// 中心未实现该端点（404/405）时按「无操作」处理——租约最晚按 TTL 过期，
// 停机流程不应因为中心版本旧而失败。
func (s *Source) Release(ctx context.Context, task *asynctask.AsyncTask) error {
	if task == nil || task.ID == "" {
		return nil
	}
	body, _ := json.Marshal(wire.ReleaseRequest{TaskID: task.ID, WorkerID: s.cfg.WorkerID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.cfg.BaseURL+wire.PathRelease, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound, http.StatusMethodNotAllowed:
		return nil
	default:
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("asynctask: remote Release status=%d body=%s", resp.StatusCode, bytes.TrimSpace(slurp))
	}
}

// Save 保存一次执行结果：先调本地钩子 callback，再 POST {base}/Callback。
//
// 具体流程：
//  1. 先调用本地钩子 callback(ctx)（远程形态下 ctx 中不含 tx，asynctask.TxFromContext 恒为 nil）。
//     钩子返回的错误不阻断回传——结果仍需上报给中心（否则中心会一直认为任务在跑）。
//  2. 无论钩子成败都 POST Callback，携带 status / result / error。
//  3. Callback 失败按 CallbackRetryDelays（默认 200ms/1s/3s）重试 CallbackMaxRetry 次；
//     仍失败则返回包装了 ErrCallbackLost 的错误，由接入方兜底
//     （中心侧可能仍认为任务在跑，需告警/人工核对）。
//
// 返回：Callback 最终成功时返回 nil（钩子错误被吞掉，仅记录在回传的 error 字段里）；
// Callback 最终失败时返回 ErrCallbackLost 包装错误。
func (s *Source) Save(ctx context.Context, task *asynctask.AsyncTask, r asynctask.Result, callback asynctask.EnsureFunc) error {
	// 远程形态：先调本地钩子（TxFromContext 为 nil）。钩子错误不阻断回传。
	var hookErr string
	if callback != nil {
		if err := callback(ctx); err != nil {
			hookErr = err.Error()
		}
	}

	errMsg := hookErr
	if r.Err != nil {
		if errMsg != "" {
			errMsg = errMsg + "; " + r.Err.Error()
		} else {
			errMsg = r.Err.Error()
		}
	}

	var taskID, workerID string
	if task != nil {
		taskID = task.ID
	}
	workerID = s.cfg.WorkerID

	body, _ := json.Marshal(wire.CallbackRequest{
		TaskID:     taskID,
		WorkerID:   workerID,
		Status:     string(r.Outcome),
		Failure:    string(r.Failure),
		Result:     r.Payload,
		Error:      errMsg,
		DurationMS: r.Duration.Milliseconds(),
	})

	return s.postCallback(ctx, body)
}

// postCallback POST Callback，按配置重试。
func (s *Source) postCallback(ctx context.Context, body []byte) error {
	url := s.cfg.BaseURL + wire.PathCallback
	var lastErr error
	attempts := s.cfg.CallbackMaxRetry + 1 // 首次 + 重试次数
	for i := 0; i < attempts; i++ {
		if i > 0 {
			// 第 i 次重试前的等待：取 CallbackRetryDelays[i-1]。
			delay := s.retryDelay(i - 1)
			if err := sleepCtx(ctx, delay); err != nil {
				return fmt.Errorf("%w: %v (context done during retry)", ErrCallbackLost, err)
			}
		}
		err := s.doCallbackOnce(ctx, url, body)
		if err == nil {
			return nil
		}
		lastErr = err
		// 上下文已取消/超时则不再重试。
		if ctx.Err() != nil {
			break
		}
	}
	return fmt.Errorf("%w: %v", ErrCallbackLost, lastErr)
}

func (s *Source) doCallbackOnce(ctx context.Context, url string, body []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		slurp, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("Callback status=%d body=%s", resp.StatusCode, bytes.TrimSpace(slurp))
	}
	return nil
}

// retryDelay 返回第 idx 次重试（0-based）前的等待时长。
func (s *Source) retryDelay(idx int) time.Duration {
	delays := s.cfg.CallbackRetryDelays
	if len(delays) == 0 {
		delays = defaultCallbackRetryDelays
	}
	if idx < len(delays) {
		return delays[idx]
	}
	return delays[len(delays)-1]
}

// Close 关闭 HTTP 连接。若 client 由本包自建，则调用 CloseIdleConnections；
// 外部传入的 client 生命周期由调用方负责，本方法不关闭它。
func (s *Source) Close(_ context.Context) error {
	if s.own && s.client != nil {
		s.client.CloseIdleConnections()
	}
	return nil
}

// NoAsyncTaskBackoff 计算「Fetch 返回 ErrNoAsyncTask 后 Worker 应等待多久」的可复用退避：
// 从 FetchNoAsyncTaskBackoffInitial（500ms）起，按失败次数指数增长，封顶 FetchNoAsyncTaskBackoffMax（5s）。
//
// 这是给 Worker 侧复用的纯函数——本包的 Fetch 不 sleep，只返回 ErrNoAsyncTask，
// 由 Worker 按本函数退避，避免把阻塞逻辑埋进 Source。
//
// n 为连续无任务的次数（从 0 开始）：n=0→500ms，n=1→1s，n=2→2s，n=3→4s，n>=4→5s。
func NoAsyncTaskBackoff(n int) time.Duration {
	if n < 0 {
		n = 0
	}
	d := time.Duration(float64(FetchNoAsyncTaskBackoffInitial) * math.Pow(2, float64(n)))
	if d <= 0 || d > FetchNoAsyncTaskBackoffMax {
		return FetchNoAsyncTaskBackoffMax
	}
	return d
}

// sleepCtx 带 ctx 取消的 sleep。
func sleepCtx(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
