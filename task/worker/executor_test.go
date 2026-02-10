package worker

import (
	"context"
	"fmt"
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
