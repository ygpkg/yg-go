package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/task/worker"
)

type DemoPayload struct {
	Message string `json:"message"`
	UserID  int    `json:"user_id"`
}

type DemoTaskExecutor struct {
	payload DemoPayload
}

func NewDemoTaskExecutor(payloadJSON string) (*DemoTaskExecutor, error) {
	var payload DemoPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("✓ 任务已初始化，参数: %+v\n", payload)
	return &DemoTaskExecutor{payload: payload}, nil
}

func (e *DemoTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("→ Execute: 开始执行任务\n")
	fmt.Printf("  处理消息: %s\n", e.payload.Message)
	fmt.Printf("  用户 ID: %d\n", e.payload.UserID)

	time.Sleep(2 * time.Second)

	fmt.Printf("✓ Execute: 任务执行完成\n")
	return nil
}

func (e *DemoTaskExecutor) GetResult() string {
	data, _ := json.Marshal(map[string]interface{}{
		"message": e.payload.Message,
		"user_id": e.payload.UserID,
		"status":  "completed",
	})
	return string(data)
}

func (e *DemoTaskExecutor) SetResult(result string) {}

func (e *DemoTaskExecutor) OnSuccess(ctx context.Context) error {
	fmt.Printf("✓ OnSuccess: 任务执行成功\n")
	return nil
}

func (e *DemoTaskExecutor) OnFailure(ctx context.Context) error {
	fmt.Printf("✗ OnFailure: 任务执行失败\n")
	return nil
}

var _ worker.TaskExecutor = (*DemoTaskExecutor)(nil)
