package asynctask

import "errors"

// 错误哨兵，前缀统一为 "asynctask: "。
//
// ErrNotify 是唯一「有副作用但报错」的哨兵：任务已落库、只是唤醒没发出去。
// 调用方按「已成功 + 稍后自查」处理，不要重试提交（重试会多建一条任务）。
var (
	// ErrNilDB 数据库连接为 nil。
	ErrNilDB = errors.New("asynctask: db is nil")
	// ErrEmptyType 任务类型为空。
	ErrEmptyType = errors.New("asynctask: task type is empty")
	// ErrNilHandler 任务处理器为 nil。
	ErrNilHandler = errors.New("asynctask: handler is nil")
	// ErrDuplicateType 同一 Worker 内重复注册同一任务类型。
	ErrDuplicateType = errors.New("asynctask: duplicate task type")
	// ErrAsyncTaskNotFound 任务不存在。
	ErrAsyncTaskNotFound = errors.New("asynctask: asyncTask not found")
	// ErrRunNotFound 尝试记录不存在。
	ErrRunNotFound = errors.New("asynctask: run not found")
	// ErrNotReplayable 任务未处于可重放的终态。
	ErrNotReplayable = errors.New("asynctask: asyncTask not replayable")
	// ErrPayloadTooLarge payload 超过 MaxPayloadBytes（不截断，直接报错）。
	ErrPayloadTooLarge = errors.New("asynctask: payload too large")
	// ErrInvalidConfig 配置非法。
	ErrInvalidConfig = errors.New("asynctask: invalid config")
	// ErrWorkerAlreadyRunning Worker 已在运行（Run 后再次 Run / Register）。
	ErrWorkerAlreadyRunning = errors.New("asynctask: worker already running")
	// ErrWorkerStopped Worker 已停机（Shutdown 后再 Run）。
	ErrWorkerStopped = errors.New("asynctask: worker stopped")
	// ErrNoType Worker 未注册任何任务类型即 Run。
	ErrNoType = errors.New("asynctask: no task type registered")
	// ErrUniqKeyRace 幂等键冲突回查为空的重试后仍失败（并发提交者回滚或副本延迟）。
	ErrUniqKeyRace = errors.New("asynctask: uniq key race")
	// ErrLeaseLost 已失去任务所有权（被接管 / 被取消 / 已进入终态）。
	//
	// 语义：Renew/Save 返回它时，调用方必须停止执行并放弃业务钩子——
	// 此时写终态会被守卫拦下，钩子里的副作用却无法回滚，故 Source 保证「失权不跑钩子」。
	ErrLeaseLost = errors.New("asynctask: lease lost")
	// ErrNotify 任务已落库但通知没发出去（不参与正确性，兜底轮询会接手）。
	ErrNotify = errors.New("asynctask: notify failed")
	// ErrNoAsyncTask 当前无待执行任务。
	ErrNoAsyncTask = errors.New("asynctask: no task available")
)
