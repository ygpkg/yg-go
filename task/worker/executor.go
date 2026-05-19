package worker

import (
	"context"
	"sync"
)

// TaskExecutor 任务执行器接口
type TaskExecutor interface {
	Execute(ctx context.Context) error

	GetResult() string

	SetResult(result string)

	OnSuccess(ctx context.Context) error

	OnFailure(ctx context.Context) error
}

// ExecutorFactory 执行器工厂函数
type ExecutorFactory func(payload string) (TaskExecutor, error)

// PreHookFunc 前置钩子函数
type PreHookFunc func(ctx context.Context) error

// executorOptions 执行器选项配置
type executorOptions struct {
	maxConcurrency int
	preHook        PreHookFunc
}

// ExecutorOption 执行器注册选项
type ExecutorOption func(*executorOptions)

// WithConcurrency 设置任务类型的最大并发数
func WithConcurrency(n int) ExecutorOption {
	return func(opts *executorOptions) {
		opts.maxConcurrency = n
	}
}

// WithPreHook 设置任务执行前置钩子
func WithPreHook(hook PreHookFunc) ExecutorOption {
	return func(opts *executorOptions) {
		opts.preHook = hook
	}
}

// ExecutorRegistry 执行器注册表
type ExecutorRegistry struct {
	mu             sync.RWMutex
	executors      map[string]ExecutorFactory
	concurrencyMap map[string]int
	preHookMap     map[string]PreHookFunc
}

// NewExecutorRegistry 创建执行器注册表
func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{
		executors:      make(map[string]ExecutorFactory),
		concurrencyMap: make(map[string]int),
		preHookMap:     make(map[string]PreHookFunc),
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

// RegisterWithOptions 注册执行器并设置选项
func (r *ExecutorRegistry) RegisterWithOptions(taskType string, factory ExecutorFactory, opts *executorOptions) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[taskType] = factory
	if opts != nil {
		r.concurrencyMap[taskType] = opts.maxConcurrency
		r.preHookMap[taskType] = opts.preHook
	}
}

// Get 获取执行器工厂
func (r *ExecutorRegistry) Get(taskType string) (ExecutorFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.executors[taskType]
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

// GetAll 获取所有任务类型
func (r *ExecutorRegistry) GetAll() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	types := make([]string, 0, len(r.executors))
	for t := range r.executors {
		types = append(types, t)
	}
	return types
}

// GetPreHook 获取指定任务类型的前置钩子
func (r *ExecutorRegistry) GetPreHook(taskType string) PreHookFunc {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.preHookMap[taskType]
}
