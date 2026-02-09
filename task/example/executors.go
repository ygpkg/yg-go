package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"
)

// ===== Payload 结构定义 =====

// DemoPayload 演示任务的参数结构
type DemoPayload struct {
	Message string `json:"message"`
	UserID  int    `json:"user_id"`
}

// RetryPayload 重试任务的参数结构
type RetryPayload struct {
	Message    string `json:"message"`
	FailTimes  int    `json:"fail_times"`  // 前几次失败
	FailReason string `json:"fail_reason"` // 失败原因
}

// TimeoutPayload 超时任务的参数结构
type TimeoutPayload struct {
	Message      string `json:"message"`
	Duration     int    `json:"duration"`      // 任务执行时长（秒）
	CheckContext bool   `json:"check_context"` // 是否检查上下文取消
}

// ConcurrentPayload 并发任务的参数结构
type ConcurrentPayload struct {
	Index    int    `json:"index"`
	Message  string `json:"message"`
	Duration int    `json:"duration"` // 任务执行时长（毫秒）
}

// StepPayload 步骤任务的参数结构
type StepPayload struct {
	StepName    string `json:"step_name"`
	OrderID     int    `json:"order_id"`
	Description string `json:"description"`
	Step        int    `json:"step"`
	AppGroup    string `json:"app_group"`
}

// FastTaskPayload 快速任务参数
type FastTaskPayload struct {
	Index    int    `json:"index"`
	Message  string `json:"message"`
	Duration int    `json:"duration"` // 执行时长（毫秒）
}

// SlowTaskPayload 慢速任务参数
type SlowTaskPayload struct {
	Index    int    `json:"index"`
	Message  string `json:"message"`
	Duration int    `json:"duration"` // 执行时长（毫秒）
}

// ApiTaskPayload API 调用任务参数
type ApiTaskPayload struct {
	Index    int    `json:"index"`
	Endpoint string `json:"endpoint"`
	Duration int    `json:"duration"` // 执行时长（毫秒）
}

// DefaultTaskPayload 默认任务参数
type DefaultTaskPayload struct {
	Index    int    `json:"index"`
	Message  string `json:"message"`
	Duration int    `json:"duration"` // 执行时长（毫秒）
}

// ===== 共享统计结构 =====

// TaskStats 任务统计信息
type TaskStats struct {
	fastExecuting    int32
	fastCompleted    int32
	slowExecuting    int32
	slowCompleted    int32
	apiExecuting     int32
	apiCompleted     int32
	defaultExecuting int32
	defaultCompleted int32
	mu               sync.Mutex
	startTimes       map[string]time.Time
}

// NewTaskStats 创建任务统计实例
func NewTaskStats() *TaskStats {
	return &TaskStats{
		startTimes: make(map[string]time.Time),
	}
}

// RecordStart 记录任务开始时间
func (s *TaskStats) RecordStart(taskType string, index int) {
	key := fmt.Sprintf("%s-%d", taskType, index)
	s.mu.Lock()
	s.startTimes[key] = time.Now()
	s.mu.Unlock()
}

// GetElapsed 获取任务执行时长
func (s *TaskStats) GetElapsed(taskType string, index int) time.Duration {
	key := fmt.Sprintf("%s-%d", taskType, index)
	s.mu.Lock()
	startTime := s.startTimes[key]
	s.mu.Unlock()
	return time.Since(startTime)
}

// ===== 1. DemoTaskExecutor - 基本示例执行器 =====

// DemoTaskExecutor 演示任务执行器
type DemoTaskExecutor struct {
	payload DemoPayload
	result  interface{}
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

	// 模拟任务处理
	time.Sleep(2 * time.Second)

	// 设置执行结果
	e.result = map[string]interface{}{
		"message": e.payload.Message,
		"user_id": e.payload.UserID,
		"status":  "completed",
	}

	fmt.Printf("✓ Execute: 任务执行完成\n")
	return nil
}

// GetResult 获取执行结果
func (e *DemoTaskExecutor) GetResult() interface{} {
	return e.result
}

// SetResult 设置执行结果
func (e *DemoTaskExecutor) SetResult(result interface{}) {
	e.result = result
}

// OnSuccess 成功回调
func (e *DemoTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("✓ OnSuccess: 任务执行成功\n")
	return nil
}

// OnFailure 失败回调
func (e *DemoTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("✗ OnFailure: 任务执行失败\n")
	return nil
}

// ===== 2. RetryTaskExecutor - 重试机制执行器 =====

// RetryTaskExecutor 演示重试机制的任务执行器
type RetryTaskExecutor struct {
	payload        RetryPayload
	attemptCount   *int32
	currentAttempt int32
	result         interface{}
}

// NewRetryTaskExecutor 创建RetryTaskExecutor
func NewRetryTaskExecutor(payloadJSON string, attemptCount *int32) (*RetryTaskExecutor, error) {
	var payload RetryPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	currentAttempt := atomic.AddInt32(attemptCount, 1)

	fmt.Printf("\n════════════════════════════════════════\n")
	fmt.Printf("准备执行任务 (第 %d 次尝试)\n", currentAttempt)
	fmt.Printf("════════════════════════════════════════\n")
	fmt.Printf("配置的失败次数: %d\n", payload.FailTimes)

	return &RetryTaskExecutor{
		payload:        payload,
		attemptCount:   attemptCount,
		currentAttempt: currentAttempt,
	}, nil
}

// Execute 执行任务
func (e *RetryTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("\n→ 开始执行任务...\n")

	time.Sleep(1 * time.Second)

	if int(e.currentAttempt) <= e.payload.FailTimes {
		errMsg := fmt.Sprintf("%s (尝试 %d/%d)",
			e.payload.FailReason,
			e.currentAttempt,
			e.payload.FailTimes)
		fmt.Printf("✗ 任务执行失败: %s\n", errMsg)
		return fmt.Errorf("%s", errMsg)
	}

	// 设置执行结果
	e.result = map[string]interface{}{
		"attempts": e.currentAttempt,
		"status":   "success",
	}

	fmt.Printf("✓ 任务执行成功 (尝试 %d 次后成功)\n", e.currentAttempt)
	return nil
}

// GetResult 获取执行结果
func (e *RetryTaskExecutor) GetResult() interface{} {
	return e.result
}

// SetResult 设置执行结果
func (e *RetryTaskExecutor) SetResult(result interface{}) {
	e.result = result
}

// OnSuccess 成功回调
func (e *RetryTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("\n✓ OnSuccess: 任务最终成功\n")
	fmt.Printf("  总尝试次数: %d\n", e.currentAttempt)
	return nil
}

// OnFailure 失败回调
func (e *RetryTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("\n✗ OnFailure: 任务执行失败\n")
	fmt.Printf("  → 任务将重试\n")
	return nil
}

// ===== 3. TimeoutTaskExecutor - 超时处理执行器 =====

// TimeoutTaskExecutor 演示超时处理的任务执行器
type TimeoutTaskExecutor struct {
	payload TimeoutPayload
	result  interface{}
}

// NewTimeoutTaskExecutor 创建TimeoutTaskExecutor
func NewTimeoutTaskExecutor(payloadJSON string) (*TimeoutTaskExecutor, error) {
	var payload TimeoutPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("\n════════════════════════════════════════\n")
	fmt.Printf("准备执行任务\n")
	fmt.Printf("════════════════════════════════════════\n")
	fmt.Printf("任务执行时长: %d 秒\n", payload.Duration)
	fmt.Printf("检查上下文取消: %v\n", payload.CheckContext)

	return &TimeoutTaskExecutor{payload: payload}, nil
}

// Execute 执行任务
func (e *TimeoutTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("\n→ 开始执行任务...\n")

	duration := time.Duration(e.payload.Duration) * time.Second

	var err error
	if e.payload.CheckContext {
		err = e.executeWithContextCheck(ctx, duration)
	} else {
		err = e.executeWithoutContextCheck(duration)
	}

	if err == nil {
		e.result = map[string]interface{}{
			"duration": e.payload.Duration,
			"status":   "completed",
		}
	}

	return err
}

// GetResult 获取执行结果
func (e *TimeoutTaskExecutor) GetResult() interface{} {
	return e.result
}

// SetResult 设置执行结果
func (e *TimeoutTaskExecutor) SetResult(result interface{}) {
	e.result = result
}

// executeWithContextCheck 执行任务并检查上下文取消
func (e *TimeoutTaskExecutor) executeWithContextCheck(ctx context.Context, duration time.Duration) error {
	fmt.Println("  使用上下文检查模式")

	deadline := time.Now().Add(duration)
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	processed := 0
	total := int(duration.Seconds())

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			fmt.Printf("\n✗ 检测到上下文取消: %v\n", ctx.Err())
			fmt.Printf("  已处理: %d/%d 项\n", processed, total)
			return ctx.Err()

		case <-ticker.C:
			processed++
			fmt.Printf("  处理进度: %d/%d\n", processed, total)
		}
	}

	fmt.Printf("\n✓ 任务执行完成 (共处理 %d 项)\n", processed)
	return nil
}

// executeWithoutContextCheck 执行任务但不检查上下文
func (e *TimeoutTaskExecutor) executeWithoutContextCheck(duration time.Duration) error {
	fmt.Println("  使用非上下文检查模式（不推荐）")
	fmt.Printf("  → 睡眠 %d 秒...\n", e.payload.Duration)

	time.Sleep(duration)

	fmt.Printf("\n✓ 任务执行完成\n")
	return nil
}

// OnSuccess 成功回调
func (e *TimeoutTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("\n✓ OnSuccess: 任务成功完成\n")
	return nil
}

// OnFailure 失败回调
func (e *TimeoutTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("\n✗ OnFailure: 任务失败\n")
	return nil
}

// ===== 4. ConcurrentTaskExecutor - 并发任务执行器 =====

// ConcurrentTaskExecutor 演示并发处理的任务执行器
type ConcurrentTaskExecutor struct {
	payload        ConcurrentPayload
	executingCount *int32
	completedCount *int32
	mu             *sync.Mutex
	startTimes     *map[int]time.Time
	startTime      time.Time
	result         interface{}
}

// NewConcurrentTaskExecutor 创建ConcurrentTaskExecutor
func NewConcurrentTaskExecutor(payloadJSON string, executingCount, completedCount *int32, mu *sync.Mutex, startTimes *map[int]time.Time) (*ConcurrentTaskExecutor, error) {
	var payload ConcurrentPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	startTime := time.Now()
	mu.Lock()
	(*startTimes)[payload.Index] = startTime
	mu.Unlock()

	return &ConcurrentTaskExecutor{
		payload:        payload,
		executingCount: executingCount,
		completedCount: completedCount,
		mu:             mu,
		startTimes:     startTimes,
		startTime:      startTime,
	}, nil
}

// Execute 执行任务
func (e *ConcurrentTaskExecutor) Execute(ctx context.Context) error {
	current := atomic.AddInt32(e.executingCount, 1)

	fmt.Printf("[任务 %d] 开始执行 (并发数: %d)\n", e.payload.Index, current)

	duration := time.Duration(e.payload.Duration) * time.Millisecond
	time.Sleep(duration)

	atomic.AddInt32(e.executingCount, -1)

	// 设置执行结果
	e.result = map[string]interface{}{
		"index":    e.payload.Index,
		"duration": e.payload.Duration,
		"status":   "completed",
	}

	fmt.Printf("[任务 %d] 执行完成 (耗时: %dms)\n", e.payload.Index, e.payload.Duration)

	return nil
}

// GetResult 获取执行结果
func (e *ConcurrentTaskExecutor) GetResult() interface{} {
	return e.result
}

// SetResult 设置执行结果
func (e *ConcurrentTaskExecutor) SetResult(result interface{}) {
	e.result = result
}

// OnSuccess 成功回调
func (e *ConcurrentTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	completed := atomic.AddInt32(e.completedCount, 1)
	elapsed := time.Since(e.startTime)

	fmt.Printf("[任务 %d] ✓ 成功 (总耗时: %v, 已完成: %d)\n",
		e.payload.Index, elapsed.Round(time.Millisecond), completed)

	return nil
}

// OnFailure 失败回调
func (e *ConcurrentTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("[任务 %d] ✗ 失败\n", e.payload.Index)
	return nil
}

// ===== 5. StepTaskExecutor - 步骤化任务执行器 =====

// StepTaskExecutor 步骤任务执行器
type StepTaskExecutor struct {
	payload        StepPayload
	executionOrder *[]int
	mu             *sync.Mutex
	result         interface{}
}

// NewStepTaskExecutor 创建StepTaskExecutor
func NewStepTaskExecutor(payloadJSON string, executionOrder *[]int, mu *sync.Mutex) (*StepTaskExecutor, error) {
	var payload StepPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}

	fmt.Printf("\n════════════════════════════════════════\n")
	fmt.Printf("准备执行步骤 %d: %s\n", payload.Step, payload.StepName)
	fmt.Printf("════════════════════════════════════════\n")
	fmt.Printf("订单 ID: %d\n", payload.OrderID)
	fmt.Printf("AppGroup: %s\n", payload.AppGroup)
	fmt.Printf("描述: %s\n", payload.Description)

	return &StepTaskExecutor{
		payload:        payload,
		executionOrder: executionOrder,
		mu:             mu,
	}, nil
}

// Execute 执行任务
func (e *StepTaskExecutor) Execute(ctx context.Context) error {
	fmt.Printf("\n→ 执行步骤 %d: %s\n", e.payload.Step, e.payload.StepName)

	e.mu.Lock()
	*e.executionOrder = append(*e.executionOrder, e.payload.Step)
	e.mu.Unlock()

	time.Sleep(2 * time.Second)

	// 设置执行结果
	e.result = map[string]interface{}{
		"step":      e.payload.Step,
		"step_name": e.payload.StepName,
		"order_id":  e.payload.OrderID,
		"status":    "completed",
	}

	fmt.Printf("✓ 步骤 %d 执行完成\n", e.payload.Step)
	return nil
}

// GetResult 获取执行结果
func (e *StepTaskExecutor) GetResult() interface{} {
	return e.result
}

// SetResult 设置执行结果
func (e *StepTaskExecutor) SetResult(result interface{}) {
	e.result = result
}

// OnSuccess 成功回调
func (e *StepTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("✓ 步骤 %d (%s) 成功\n", e.payload.Step, e.payload.StepName)
	return nil
}

// OnFailure 失败回调
func (e *StepTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("✗ 步骤 %d (%s) 失败\n", e.payload.Step, e.payload.StepName)
	return nil
}

// ===== 6. 混合并发执行器组 =====

// FastTaskExecutor 快速任务执行器（高并发）
type FastTaskExecutor struct {
	payload FastTaskPayload
	stats   *TaskStats
	result  interface{}
}

func NewFastTaskExecutor(payloadJSON string, stats *TaskStats) (*FastTaskExecutor, error) {
	var payload FastTaskPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}
	stats.RecordStart("fast", payload.Index)
	return &FastTaskExecutor{payload: payload, stats: stats}, nil
}

func (e *FastTaskExecutor) Execute(ctx context.Context) error {
	current := atomic.AddInt32(&e.stats.fastExecuting, 1)
	fmt.Printf("  [快速任务 %d] 开始执行 (当前并发: %d)\n", e.payload.Index, current)

	time.Sleep(time.Duration(e.payload.Duration) * time.Millisecond)

	atomic.AddInt32(&e.stats.fastExecuting, -1)

	// 设置执行结果
	e.result = map[string]interface{}{
		"index":    e.payload.Index,
		"duration": e.payload.Duration,
		"type":     "fast",
	}

	return nil
}

func (e *FastTaskExecutor) GetResult() interface{} {
	return e.result
}

func (e *FastTaskExecutor) SetResult(result interface{}) {
	e.result = result
}

func (e *FastTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	completed := atomic.AddInt32(&e.stats.fastCompleted, 1)
	elapsed := e.stats.GetElapsed("fast", e.payload.Index)
	fmt.Printf("  [快速任务 %d] ✓ 完成 (耗时: %v, 已完成: %d)\n",
		e.payload.Index, elapsed.Round(time.Millisecond), completed)
	return nil
}

func (e *FastTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("  [快速任务 %d] ✗ 失败\n", e.payload.Index)
	return nil
}

// SlowTaskExecutor 慢速任务执行器（低并发）
type SlowTaskExecutor struct {
	payload SlowTaskPayload
	stats   *TaskStats
	result  interface{}
}

func NewSlowTaskExecutor(payloadJSON string, stats *TaskStats) (*SlowTaskExecutor, error) {
	var payload SlowTaskPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}
	stats.RecordStart("slow", payload.Index)
	return &SlowTaskExecutor{payload: payload, stats: stats}, nil
}

func (e *SlowTaskExecutor) Execute(ctx context.Context) error {
	current := atomic.AddInt32(&e.stats.slowExecuting, 1)
	fmt.Printf("  [慢速任务 %d] 开始执行 (当前并发: %d)\n", e.payload.Index, current)

	time.Sleep(time.Duration(e.payload.Duration) * time.Millisecond)

	atomic.AddInt32(&e.stats.slowExecuting, -1)

	// 设置执行结果
	e.result = map[string]interface{}{
		"index":    e.payload.Index,
		"duration": e.payload.Duration,
		"type":     "slow",
	}

	return nil
}

func (e *SlowTaskExecutor) GetResult() interface{} {
	return e.result
}

func (e *SlowTaskExecutor) SetResult(result interface{}) {
	e.result = result
}

func (e *SlowTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	completed := atomic.AddInt32(&e.stats.slowCompleted, 1)
	elapsed := e.stats.GetElapsed("slow", e.payload.Index)
	fmt.Printf("  [慢速任务 %d] ✓ 完成 (耗时: %v, 已完成: %d)\n",
		e.payload.Index, elapsed.Round(time.Millisecond), completed)
	return nil
}

func (e *SlowTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("  [慢速任务 %d] ✗ 失败\n", e.payload.Index)
	return nil
}

// ApiTaskExecutor API 调用任务执行器（中等并发）
type ApiTaskExecutor struct {
	payload ApiTaskPayload
	stats   *TaskStats
	result  interface{}
}

func NewApiTaskExecutor(payloadJSON string, stats *TaskStats) (*ApiTaskExecutor, error) {
	var payload ApiTaskPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}
	stats.RecordStart("api", payload.Index)
	return &ApiTaskExecutor{payload: payload, stats: stats}, nil
}

func (e *ApiTaskExecutor) Execute(ctx context.Context) error {
	current := atomic.AddInt32(&e.stats.apiExecuting, 1)
	fmt.Printf("  [API任务 %d] 开始执行 (当前并发: %d)\n", e.payload.Index, current)

	time.Sleep(time.Duration(e.payload.Duration) * time.Millisecond)

	atomic.AddInt32(&e.stats.apiExecuting, -1)

	// 设置执行结果
	e.result = map[string]interface{}{
		"index":    e.payload.Index,
		"endpoint": e.payload.Endpoint,
		"duration": e.payload.Duration,
		"type":     "api",
	}

	return nil
}

func (e *ApiTaskExecutor) GetResult() interface{} {
	return e.result
}

func (e *ApiTaskExecutor) SetResult(result interface{}) {
	e.result = result
}

func (e *ApiTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	completed := atomic.AddInt32(&e.stats.apiCompleted, 1)
	elapsed := e.stats.GetElapsed("api", e.payload.Index)
	fmt.Printf("  [API任务 %d] ✓ 完成 (耗时: %v, 已完成: %d)\n",
		e.payload.Index, elapsed.Round(time.Millisecond), completed)
	return nil
}

func (e *ApiTaskExecutor) OnFailure(ctx context.Context, tx *gorm.DB) error {
	fmt.Printf("  [API任务 %d] ✗ 失败\n", e.payload.Index)
	return nil
}

// DefaultTaskExecutor 默认任务执行器（使用全局并发数）
type DefaultTaskExecutor struct {
	payload DefaultTaskPayload
	stats   *TaskStats
	result  interface{}
}

func NewDefaultTaskExecutor(payloadJSON string, stats *TaskStats) (*DefaultTaskExecutor, error) {
	var payload DefaultTaskPayload
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return nil, fmt.Errorf("failed to parse payload: %w", err)
	}
	stats.RecordStart("default", payload.Index)
	return &DefaultTaskExecutor{payload: payload, stats: stats}, nil
}

func (e *DefaultTaskExecutor) Execute(ctx context.Context) error {
	current := atomic.AddInt32(&e.stats.defaultExecuting, 1)
	fmt.Printf("  [默认任务 %d] 开始执行 (当前并发: %d)\n", e.payload.Index, current)

	time.Sleep(time.Duration(e.payload.Duration) * time.Millisecond)

	atomic.AddInt32(&e.stats.defaultExecuting, -1)
	return nil
}

func (e *DefaultTaskExecutor) OnSuccess(ctx context.Context, tx *gorm.DB) error {
	completed := atomic.AddInt32(&e.stats.defaultCompleted, 1)
	elapsed := e.stats.GetElapsed("default", e.payload.Index)
	fmt.Printf("  [默认任务 %d] ✓ 完成 (耗时: %v, 已完成: %d)\n",
		e.payload.Index, elapsed.Round(time.Millisecond), completed)
	return nil
}
