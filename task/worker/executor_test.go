package worker

import (
	"context"
	"fmt"
	"sync"
	"testing"
)

// mockExecutor 模拟执行器
type mockExecutor struct {
	payload         string
	executeCalled   bool
	executeError    error
	onSuccessCalled bool
	onFailureCalled bool
	result          interface{}
}

func newMockExecutor(payload string) (*mockExecutor, error) {
	if payload == "error" {
		return nil, fmt.Errorf("failed to create executor")
	}
	return &mockExecutor{
		payload: payload,
	}, nil
}

func (m *mockExecutor) Execute(ctx context.Context) error {
	m.executeCalled = true
	return m.executeError
}

func (m *mockExecutor) GetResult() interface{} {
	return m.result
}

func (m *mockExecutor) SetResult(result interface{}) {
	m.result = result
}

func (m *mockExecutor) OnSuccess(ctx context.Context) error {
	m.onSuccessCalled = true
	return nil
}

func (m *mockExecutor) OnFailure(ctx context.Context) error {
	m.onFailureCalled = true
	return nil
}

func TestExecutorRegistry_Register(t *testing.T) {
	registry := NewExecutorRegistry()

	factory := func(payload string) (TaskExecutor, error) {
		return newMockExecutor(payload)
	}

	registry.Register("test_task", factory)

	// 验证注册成功
	retrievedFactory, ok := registry.Get("test_task")
	if !ok {
		t.Error("Expected factory to be registered")
	}

	// 验证工厂函数可以创建执行器
	executor, err := retrievedFactory("test_payload")
	if err != nil {
		t.Errorf("Expected factory to create executor without error, got: %v", err)
	}
	if executor == nil {
		t.Error("Expected factory to create executor")
	}

	if mock, ok := executor.(*mockExecutor); !ok {
		t.Error("Expected executor to be mockExecutor")
	} else if mock.payload != "test_payload" {
		t.Errorf("Expected payload to be 'test_payload', got %s", mock.payload)
	}
}

func TestExecutorRegistry_GetNotFound(t *testing.T) {
	registry := NewExecutorRegistry()

	_, ok := registry.Get("nonexistent")
	if ok {
		t.Error("Expected factory not to be found")
	}
}

func TestExecutorRegistry_GetAll(t *testing.T) {
	registry := NewExecutorRegistry()

	factory := func(payload string) (TaskExecutor, error) {
		return newMockExecutor(payload)
	}

	registry.Register("task1", factory)
	registry.Register("task2", factory)
	registry.Register("task3", factory)

	types := registry.GetAll()
	if len(types) != 3 {
		t.Errorf("Expected 3 task types, got %d", len(types))
	}

	// 验证包含所有任务类型
	typeMap := make(map[string]bool)
	for _, t := range types {
		typeMap[t] = true
	}

	if !typeMap["task1"] || !typeMap["task2"] || !typeMap["task3"] {
		t.Error("Expected all task types to be present")
	}
}

func TestExecutorFactory_CreateError(t *testing.T) {
	registry := NewExecutorRegistry()

	factory := func(payload string) (TaskExecutor, error) {
		return newMockExecutor(payload)
	}

	registry.Register("test_task", factory)

	// 测试创建失败的情况
	retrievedFactory, _ := registry.Get("test_task")
	_, err := retrievedFactory("error")
	if err == nil {
		t.Error("Expected error when creating executor with 'error' payload")
	}
}

func TestMockExecutor_Execute(t *testing.T) {
	executor := &mockExecutor{}

	err := executor.Execute(context.Background())
	if err != nil {
		t.Errorf("Execute() error = %v", err)
	}

	if !executor.executeCalled {
		t.Error("Expected Execute to be called")
	}
}

func TestMockExecutor_Callbacks(t *testing.T) {
	executor := &mockExecutor{}

	// 测试 OnSuccess
	err := executor.OnSuccess(context.Background())
	if err != nil {
		t.Errorf("OnSuccess() error = %v", err)
	}
	if !executor.onSuccessCalled {
		t.Error("Expected OnSuccess to be called")
	}

	// 测试 OnFailure
	err = executor.OnFailure(context.Background())
	if err != nil {
		t.Errorf("OnFailure() error = %v", err)
	}
	if !executor.onFailureCalled {
		t.Error("Expected OnFailure to be called")
	}
}

// TestExecutorRegistry_RegisterWithConcurrency 测试注册执行器并设置并发数
func TestExecutorRegistry_RegisterWithConcurrency(t *testing.T) {
	registry := NewExecutorRegistry()

	factory := func(payload string) (TaskExecutor, error) {
		return newMockExecutor(payload)
	}

	registry.RegisterWithConcurrency("test_task", factory, 10)

	// 验证注册成功
	retrievedFactory, ok := registry.Get("test_task")
	if !ok {
		t.Error("Expected factory to be registered")
	}

	// 验证工厂函数可以创建执行器
	executor, err := retrievedFactory("test_payload")
	if err != nil {
		t.Errorf("Expected factory to create executor without error, got: %v", err)
	}
	if executor == nil {
		t.Error("Expected factory to create executor")
	}

	// 验证并发数
	concurrency := registry.GetConcurrency("test_task", 5)
	if concurrency != 10 {
		t.Errorf("Expected concurrency to be 10, got %d", concurrency)
	}
}

// TestExecutorRegistry_GetConcurrency_Default 测试获取默认并发数
func TestExecutorRegistry_GetConcurrency_Default(t *testing.T) {
	registry := NewExecutorRegistry()

	factory := func(payload string) (TaskExecutor, error) {
		return newMockExecutor(payload)
	}

	// 只注册，不设置并发数
	registry.Register("test_task", factory)

	// 应该返回默认值
	concurrency := registry.GetConcurrency("test_task", 5)
	if concurrency != 5 {
		t.Errorf("Expected default concurrency to be 5, got %d", concurrency)
	}

	// 不存在的任务类型也应该返回默认值
	concurrency = registry.GetConcurrency("nonexistent", 3)
	if concurrency != 3 {
		t.Errorf("Expected default concurrency to be 3, got %d", concurrency)
	}
}

// TestExecutorRegistry_Concurrency 测试并发注册和获取
func TestExecutorRegistry_Concurrency(t *testing.T) {
	registry := NewExecutorRegistry()

	factory := func(payload string) (TaskExecutor, error) {
		return newMockExecutor(payload)
	}

	const numGoroutines = 100
	var wg sync.WaitGroup

	// 并发注册
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			taskType := fmt.Sprintf("task_%d", idx)
			registry.RegisterWithConcurrency(taskType, factory, idx)
		}(i)
	}
	wg.Wait()

	// 并发读取
	errors := make(chan error, numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			taskType := fmt.Sprintf("task_%d", idx)

			// 验证可以获取工厂
			if _, ok := registry.Get(taskType); !ok {
				errors <- fmt.Errorf("task_%d not found", idx)
				return
			}

			// 验证并发数正确
			concurrency := registry.GetConcurrency(taskType, 10)
			if concurrency != idx {
				errors <- fmt.Errorf("expected concurrency %d, got %d", idx, concurrency)
				return
			}
		}(i)
	}
	wg.Wait()

	close(errors)

	// 检查是否有错误
	for err := range errors {
		t.Error(err)
	}

	// 验证所有任务类型都已注册
	types := registry.GetAll()
	if len(types) != numGoroutines {
		t.Errorf("Expected %d task types, got %d", numGoroutines, len(types))
	}
}
