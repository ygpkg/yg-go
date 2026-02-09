package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/task"
	"gorm.io/gorm"
)

// DemoTaskExecutor 演示任务执行器
type DemoTaskExecutor struct {
	task.BaseExecutor
	payload DemoPayload
}

// OnStart 初始化执行器
func (e *DemoTaskExecutor) OnStart(ctx context.Context, taskEntity *task.TaskEntity) error {
	// 调用基类 OnStart
	if err := e.BaseExecutor.OnStart(ctx, taskEntity); err != nil {
		return err
	}

	// 解析任务参数
	if err := json.Unmarshal([]byte(taskEntity.Payload), &e.payload); err != nil {
		return fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("✓ OnStart: 任务 %d 已初始化，参数: %+v\n", taskEntity.ID, e.payload)
	return nil
}

// Execute 执行任务
func (e *DemoTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("→ Execute: 开始执行任务 %d\n", e.Task.ID)
	fmt.Printf("  处理消息: %s\n", e.payload.Message)
	fmt.Printf("  用户 ID: %d\n", e.payload.UserID)

	// 模拟任务处理
	time.Sleep(2 * time.Second)

	fmt.Printf("✓ Execute: 任务 %d 执行完成\n", e.Task.ID)
	return nil
}

// OnSuccess 成功回调
func (e *DemoTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("✓ OnSuccess: 任务 %d 执行成功\n", e.Task.ID)
	// 这里可以执行事务性操作，如更新数据库
	return nil
}

// OnFailure 失败回调
func (e *DemoTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("✗ OnFailure: 任务 %d 执行失败\n", e.Task.ID)
	// 这里可以执行清理操作或记录日志
	return nil
}
