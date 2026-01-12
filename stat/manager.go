package stat

import (
	"context"
	"fmt"
	"sync"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sync/semaphore"
)

// MetricValue 统一的指标值接口
type MetricValue interface {
	GetIntValue() int64
	GetFloatValue() float64
}

// IntMetric 整数类型指标
type IntMetric struct {
	Value int64
}

func (m IntMetric) GetIntValue() int64 {
	return m.Value
}

func (m IntMetric) GetFloatValue() float64 {
	return float64(m.Value)
}

// FloatMetric 浮点数类型指标
type FloatMetric struct {
	Value float64
}

func (m FloatMetric) GetIntValue() int64 {
	return int64(m.Value)
}

func (m FloatMetric) GetFloatValue() float64 {
	return m.Value
}

// GroupKey 分组键的约束
type GroupKey interface {
	comparable
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~string
}

// Query 查询接口，所有查询类型都需要实现此接口
type Query interface {
	// Validate 验证查询参数的合法性
	Validate() error
}

// StatFunc 统计函数签名，支持单值和分组两种模式
// 返回 (单个值, 分组值map, error)
// - 如果是单值统计：返回 (value, nil, nil)
// - 如果是分组统计：返回 (nil, map, nil)
// - 如果出错：返回 (nil, nil, error)
//
// 注意：不应同时返回单值和分组值，Execute 方法会验证并返回错误
type StatFunc[Q Query, K GroupKey] func(ctx context.Context, query Q) (MetricValue, map[K]MetricValue, error)

// StatResult 单个统计函数的执行结果
type StatResult[K GroupKey] struct {
	Name         string
	SingleValue  MetricValue
	GroupedValue map[K]MetricValue
	IsGrouped    bool
	Error        error
}

// IsSuccess 判断是否执行成功
func (r StatResult[K]) IsSuccess() bool {
	return r.Error == nil
}

// GetSingleValue 获取单值，如果不是单值类型则返回 nil
func (r StatResult[K]) GetSingleValue() MetricValue {
	if r.IsGrouped || r.Error != nil {
		return nil
	}
	return r.SingleValue
}

// GetGroupedValue 获取分组值，如果不是分组类型则返回 nil
func (r StatResult[K]) GetGroupedValue() map[K]MetricValue {
	if !r.IsGrouped || r.Error != nil {
		return nil
	}
	return r.GroupedValue
}

// StatManager 统计管理器（泛型）
// Q: 查询类型
// K: 分组键类型
type StatManager[Q Query, K GroupKey] struct {
	funcs map[string]StatFunc[Q, K]
	mu    sync.RWMutex
}

// NewStatManager 创建统计管理器
func NewStatManager[Q Query, K GroupKey]() *StatManager[Q, K] {
	return &StatManager[Q, K]{
		funcs: make(map[string]StatFunc[Q, K]),
	}
}

// Register 注册统计函数
func (m *StatManager[Q, K]) Register(name string, fn StatFunc[Q, K]) error {
	if name == "" {
		return fmt.Errorf("stat name cannot be empty")
	}
	if fn == nil {
		return fmt.Errorf("stat function cannot be nil")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if _, exists := m.funcs[name]; exists {
		return fmt.Errorf("stat %s already registered", name)
	}

	m.funcs[name] = fn
	return nil
}

// BatchRegister 批量注册统计函数
func (m *StatManager[Q, K]) BatchRegister(funcs map[string]StatFunc[Q, K]) error {
	for name, fn := range funcs {
		if err := m.Register(name, fn); err != nil {
			return err
		}
	}
	return nil
}

// Execute 并发执行所有统计函数，最大并发数为 15
// 如果任何统计函数失败，会返回错误，但 result 仍包含成功执行的结果
// 失败的统计函数会在 result 中标记 Error 字段
func (m *StatManager[Q, K]) Execute(ctx context.Context, query Q) (map[string]StatResult[K], error) {
	// 先验证查询参数
	if err := query.Validate(); err != nil {
		return nil, fmt.Errorf("invalid query: %w", err)
	}

	m.mu.RLock()
	funcs := make(map[string]StatFunc[Q, K], len(m.funcs))
	for name, fn := range m.funcs {
		funcs[name] = fn
	}
	m.mu.RUnlock()

	if len(funcs) == 0 {
		return make(map[string]StatResult[K]), nil
	}

	result := make(map[string]StatResult[K], len(funcs))
	resultMu := sync.Mutex{}

	g, gCtx := errgroup.WithContext(ctx)
	sem := semaphore.NewWeighted(15)

	for key, v := range funcs {
		name, fn := key, v

		g.Go(func() error {
			if err := sem.Acquire(gCtx, 1); err != nil {
				return fmt.Errorf("failed to acquire semaphore for %s: %w", name, err)
			}
			defer sem.Release(1)

			singleValue, groupedValue, err := fn(gCtx, query)

			resultMu.Lock()
			defer resultMu.Unlock()

			if err != nil {
				result[name] = StatResult[K]{
					Name:  name,
					Error: err,
				}
				return fmt.Errorf("stat function %s failed: %w", name, err)
			}

			// 验证返回值的合法性：不应同时返回单值和分组值
			if singleValue != nil && groupedValue != nil {
				result[name] = StatResult[K]{
					Name:  name,
					Error: fmt.Errorf("stat function returned both single and grouped values"),
				}
				return fmt.Errorf("stat function %s returned invalid result: both single and grouped values present", name)
			}

			isGrouped := groupedValue != nil
			result[name] = StatResult[K]{
				Name:         name,
				SingleValue:  singleValue,
				GroupedValue: groupedValue,
				IsGrouped:    isGrouped,
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return result, err
	}

	return result, nil
}
