package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/task/health"
)

// ===== Payload 结构定义 =====

// DemoPayload 演示任务的参数结构
type DemoPayload struct {
	Message string `json:"message"`
	UserID  int    `json:"user_id"`
}

// ===== DemoTaskExecutor - 基本示例执行器 =====

// DemoTaskExecutor 演示任务执行器
type DemoTaskExecutor struct {
	payload DemoPayload
}

// NewDemoTaskExecutor 创建DemoTaskExecutor
func NewDemoTaskExecutor(payloadJSON string) (*DemoTaskExecutor, error) {
	var payload DemoPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("✓ 任务已初始化，参数: %+v\n", payload)
	return &DemoTaskExecutor{payload: payload}, nil
}

// Execute 执行任务
func (e *DemoTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("→ Execute: 开始执行任务\n")
	fmt.Printf("  处理消息: %s\n", e.payload.Message)
	fmt.Printf("  用户 ID: %d\n", e.payload.UserID)

	time.Sleep(2 * time.Second)

	fmt.Printf("✓ Execute: 任务执行完成\n")
	return nil
}

// GetResult 获取执行结果
func (e *DemoTaskExecutor) GetResult() string {
	data, _ := json.Marshal(map[string]interface{}{
		"message": e.payload.Message,
		"user_id": e.payload.UserID,
		"status":  "completed",
	})
	return string(data)
}

// SetResult 设置执行结果
func (e *DemoTaskExecutor) SetResult(result string) {
}

// OnSuccess 成功回调
func (e *DemoTaskExecutor) OnSuccess(ctx context.Context) error {
	fmt.Printf("✓ OnSuccess: 任务执行成功\n")
	return nil
}

// OnFailure 失败回调
func (e *DemoTaskExecutor) OnFailure(ctx context.Context) error {
	fmt.Printf("✗ OnFailure: 任务执行失败\n")
	return nil
}

// ===== TimeoutTaskExecutor - 超时任务执行器 =====

// TimeoutTaskExecutor 超时任务执行器
type TimeoutTaskExecutor struct {
	payload DemoPayload
}

// NewTimeoutTaskExecutor 创建 TimeoutTaskExecutor
func NewTimeoutTaskExecutor(payloadJSON string) (*TimeoutTaskExecutor, error) {
	var payload DemoPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("✓ 超时任务已初始化，参数: %+v\n", payload)
	return &TimeoutTaskExecutor{payload: payload}, nil
}

// Execute 执行超时任务
func (e *TimeoutTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("→ Execute: 开始执行超时任务（预计超时）\n")
	fmt.Printf("  处理消息: %s\n", e.payload.Message)
	fmt.Printf("  用户 ID: %d\n", e.payload.UserID)

	time.Sleep(70 * time.Second)

	fmt.Printf("✓ Execute: 超时任务执行完成\n")
	return nil
}

// GetResult 获取执行结果
func (e *TimeoutTaskExecutor) GetResult() string {
	data, _ := json.Marshal(map[string]interface{}{
		"message": e.payload.Message,
		"user_id": e.payload.UserID,
		"status":  "timeout_demo",
	})
	return string(data)
}

// SetResult 设置执行结果
func (e *TimeoutTaskExecutor) SetResult(result string) {
}

// OnSuccess 成功回调
func (e *TimeoutTaskExecutor) OnSuccess(ctx context.Context) error {
	fmt.Printf("✓ OnSuccess: 超时任务执行成功\n")
	return nil
}

// OnFailure 失败回调
func (e *TimeoutTaskExecutor) OnFailure(ctx context.Context) error {
	fmt.Printf("✗ OnFailure: 超时任务执行失败\n")
	return nil
}

// ===== FailTaskExecutor - 失败任务执行器 =====

// FailTaskExecutor 失败任务执行器
type FailTaskExecutor struct {
	payload DemoPayload
}

// NewFailTaskExecutor 创建 FailTaskExecutor
func NewFailTaskExecutor(payloadJSON string) (*FailTaskExecutor, error) {
	var payload DemoPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("✓ 失败任务已初始化，参数: %+v\n", payload)
	return &FailTaskExecutor{payload: payload}, nil
}

// Execute 执行失败任务
func (e *FailTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("→ Execute: 开始执行失败任务\n")
	fmt.Printf("  处理消息: %s\n", e.payload.Message)
	fmt.Printf("  用户 ID: %d\n", e.payload.UserID)

	time.Sleep(1 * time.Second)

	return fmt.Errorf("模拟任务执行失败")
}

// GetResult 获取执行结果
func (e *FailTaskExecutor) GetResult() string {
	data, _ := json.Marshal(map[string]interface{}{
		"message": e.payload.Message,
		"user_id": e.payload.UserID,
		"status":  "failed",
	})
	return string(data)
}

// SetResult 设置执行结果
func (e *FailTaskExecutor) SetResult(result string) {
}

// OnSuccess 成功回调
func (e *FailTaskExecutor) OnSuccess(ctx context.Context) error {
	fmt.Printf("✓ OnSuccess: 失败任务执行成功\n")
	return nil
}

// OnFailure 失败回调
func (e *FailTaskExecutor) OnFailure(ctx context.Context) error {
	fmt.Printf("✗ OnFailure: 失败任务执行失败\n")
	return nil
}

// ===== HealthTaskExecutor - 健康检查任务执行器 =====

// HealthTaskExecutor 健康检查任务执行器
type HealthTaskExecutor struct {
	payload       DemoPayload
	healthChecker *health.Checker
	taskType      string
	workerID      string
	taskID        uint
	stopHeartbeat chan struct{}
}

// NewHealthTaskExecutor 创建 HealthTaskExecutor
func NewHealthTaskExecutor(payloadJSON string, healthChecker *health.Checker, taskType, workerID string, taskID uint) (*HealthTaskExecutor, error) {
	var payload DemoPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("✓ 健康检查任务已初始化，参数: %+v\n", payload)
	return &HealthTaskExecutor{
		payload:       payload,
		healthChecker: healthChecker,
		taskType:      taskType,
		workerID:      workerID,
		taskID:        taskID,
		stopHeartbeat: make(chan struct{}),
	}, nil
}

// Execute 执行健康检查任务
func (e *HealthTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("→ Execute: 开始执行健康检查任务\n")
	fmt.Printf("  处理消息: %s\n", e.payload.Message)
	fmt.Printf("  用户 ID: %d\n", e.payload.UserID)

	e.startHeartbeat(ctx)

	fmt.Println("→ Execute: 任务执行 10 秒...")
	time.Sleep(10 * time.Second)

	fmt.Println("→ Execute: 停止更新心跳（模拟 Worker 崩溃）")
	close(e.stopHeartbeat)

	fmt.Println("→ Execute: 继续执行，等待健康检查器检测到超时...")
	time.Sleep(35 * time.Second)

	fmt.Printf("✓ Execute: 健康检查任务执行完成\n")
	return nil
}

// GetResult 获取执行结果
func (e *HealthTaskExecutor) GetResult() string {
	data, _ := json.Marshal(map[string]interface{}{
		"message": e.payload.Message,
		"user_id": e.payload.UserID,
		"status":  "health_check_demo",
	})
	return string(data)
}

// SetResult 设置执行结果
func (e *HealthTaskExecutor) SetResult(result string) {
}

// OnSuccess 成功回调
func (e *HealthTaskExecutor) OnSuccess(ctx context.Context) error {
	fmt.Printf("✓ OnSuccess: 健康检查任务执行成功\n")
	return nil
}

// OnFailure 失败回调
func (e *HealthTaskExecutor) OnFailure(ctx context.Context) error {
	fmt.Printf("✗ OnFailure: 健康检查任务执行失败\n")
	return nil
}

// startHeartbeat 启动心跳
func (e *HealthTaskExecutor) startHeartbeat(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-e.stopHeartbeat:
				fmt.Println("  [心跳] 停止更新")
				return
			case <-ticker.C:
				if err := e.healthChecker.SetHeartbeat(ctx, e.taskType, e.workerID, e.taskID); err != nil {
					fmt.Printf("  [心跳] 失败: %v\n", err)
				} else {
					fmt.Println("  [心跳] 已更新")
				}
			}
		}
	}()
}
