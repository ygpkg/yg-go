package worker

import (
	"context"
	"sync"

	"gorm.io/gorm"
)

// TaskExecutor 任务执行器接口
type TaskExecutor interface {
	// Execute 执行任务，包含任务的核心业务逻辑
	Execute(ctx context.Context) error

	// GetResult 获取执行结果
	GetResult() interface{}

	// SetResult 设置执行结果
	SetResult(result interface{})

	// OnSuccess 成功后回调，在任务执行成功后调用，可用于清理资源、更新状态等
	// tx 为数据库事务，如果返回错误，事务会回滚
	OnSuccess(ctx context.Context, tx *gorm.DB) error

	// OnFailure 失败后回调，在任务执行失败后调用，可用于清理资源、记录日志等
	// tx 为数据库事务，如果返回错误，事务会回滚
	OnFailure(ctx context.Context, tx *gorm.DB) error
}

// ExecutorFactory 执行器工厂函数
// 接受payload参数，由业务层在注册时决定如何解析
type ExecutorFactory func(payload string) (TaskExecutor, error)

// ExecutorRegistry 执行器注册表
// 用于管理任务类型和执行器工厂的映射关系
type ExecutorRegistry struct {
	mu        sync.RWMutex
	executors map[string]ExecutorFactory
}

// NewExecutorRegistry 创建执行器注册表
func NewExecutorRegistry() *ExecutorRegistry {
	return &ExecutorRegistry{
		executors: make(map[string]ExecutorFactory),
	}
}

// Register 注册执行器
func (r *ExecutorRegistry) Register(taskType string, factory ExecutorFactory) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.executors[taskType] = factory
}

// Get 获取执行器工厂
func (r *ExecutorRegistry) Get(taskType string) (ExecutorFactory, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	factory, ok := r.executors[taskType]
	return factory, ok
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
