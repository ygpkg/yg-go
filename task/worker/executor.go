package worker

import (
	"context"
	"sync"
)

// TaskExecutor 任务执行器接口
type TaskExecutor interface {
	// Execute 执行任务，包含任务的核心业务逻辑
	Execute(ctx context.Context) error

	// GetResult 获取执行结果
	GetResult() string

	// SetResult 设置执行结果
	SetResult(result string)

	// OnSuccess 成功后回调，在任务执行成功后调用，可用于清理资源、更新状态等
	// 注意：回调在任务状态保存后执行，如需数据库事务请自行创建
	OnSuccess(ctx context.Context) error

	// OnFailure 失败后回调，在任务执行失败后调用，可用于清理资源、记录日志等
	// 注意：回调在任务状态保存后执行，如需数据库事务请自行创建
	OnFailure(ctx context.Context) error
}

// ExecutorFactory 执行器工厂函数
// 接受payload参数，由业务层在注册时决定如何解析
type ExecutorFactory func(payload string) (TaskExecutor, error)

// ExecutorFactoryWithInfo 执行器工厂函数（携带完整任务信息）
// 相比 ExecutorFactory 额外拿到 TaskID、Timeout 等字段，便于执行器做续租、
// 幂等键等依赖任务身份的逻辑。注册该工厂后，executeTask 会优先使用它，
// 未注册时回退到旧的 ExecutorFactory 路径。
type ExecutorFactoryWithInfo func(info TaskInfo) (TaskExecutor, error)

// ExecutorRegistry 执行器注册表
// 用于管理任务类型和执行器工厂的映射关系
type ExecutorRegistry struct {
	mu             sync.RWMutex
	executors      map[string]ExecutorFactory
	executorsInfo  map[string]ExecutorFactoryWithInfo
	concurrencyMap map[string]int // 任务类型 -> 并发数映射
}

// NewExecutorRegistry 创建执行器注册表
func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{
		executors:      make(map[string]ExecutorFactory),
		executorsInfo:  make(map[string]ExecutorFactoryWithInfo),
		concurrencyMap: make(map[string]int),
	}
}

// Register 注册执行器
func (r *ExecutorRegistry) Register(taskType string, factory ExecutorFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[taskType] = factory
}

// RegisterWithConcurrency 注册执行器并设置并发数
func (r *ExecutorRegistry) RegisterWithConcurrency(taskType string, factory ExecutorFactory, concurrency int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[taskType] = factory
	r.concurrencyMap[taskType] = concurrency
}

// RegisterWithInfo 注册携带完整任务信息的执行器工厂
// 同一任务类型同时存在两种工厂时，Info 版优先（见 GetWithInfo）。
func (r *ExecutorRegistry) RegisterWithInfo(taskType string, factory ExecutorFactoryWithInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executorsInfo[taskType] = factory
}

// RegisterWithInfoAndConcurrency 注册携带完整任务信息的执行器工厂并设置并发数
func (r *ExecutorRegistry) RegisterWithInfoAndConcurrency(taskType string, factory ExecutorFactoryWithInfo, concurrency int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executorsInfo[taskType] = factory
	r.concurrencyMap[taskType] = concurrency
}

// Get 获取执行器工厂
func (r *ExecutorRegistry) Get(taskType string) (ExecutorFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.executors[taskType]
	return factory, ok
}

// GetWithInfo 获取携带完整任务信息的执行器工厂
func (r *ExecutorRegistry) GetWithInfo(taskType string) (ExecutorFactoryWithInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.executorsInfo[taskType]
	return factory, ok
}

// GetConcurrency 获取任务类型的并发数
func (r *ExecutorRegistry) GetConcurrency(taskType string, defaultConcurrency int) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if concurrency, ok := r.concurrencyMap[taskType]; ok {
		return concurrency
	}
	return defaultConcurrency
}

// GetAll 获取所有任务类型（含仅注册了 Info 版工厂的类型）
func (r *ExecutorRegistry) GetAll() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.executors)+len(r.executorsInfo))
	seen := make(map[string]struct{}, len(r.executors)+len(r.executorsInfo))
	for t := range r.executors {
		types = append(types, t)
		seen[t] = struct{}{}
	}
	for t := range r.executorsInfo {
		if _, ok := seen[t]; ok {
			continue
		}
		types = append(types, t)
	}
	return types
}
