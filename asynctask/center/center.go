// Package center 是 asynctask 的「远程任务中心」服务端参考实现。
//
// 它把 manager + source/db 适配成 source/remote 需要的四个操作：
// 代表远端 worker 认领（Fetch）、续租（Heartbeat）、回传结果（Callback）、放租约（Release）。
// 只依赖 manager 与根包，不引 gin / net/http——HTTP 适配层由接入方负责，
// 传输格式在 asynctask/wire，三者组合即一套可运行的远程形态。
//
// 中心侧必须自己保证的三件事（本实现的做法）：
//   - 同一任务同一时刻只派给一个 worker：认领走 DB 的 FOR UPDATE SKIP LOCKED 事务；
//   - Heartbeat 校验归属：非持有者的心跳直接拒绝（ErrLeaseLost）；
//   - Callback 只接受当前归属者的结果，已终态的任务按幂等 ACK 处理。
package center

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ygpkg/yg-go/asynctask"
	"github.com/ygpkg/yg-go/asynctask/manager"
	"github.com/ygpkg/yg-go/asynctask/wire"
)

// Center 是远程任务中心的服务端门面。
type Center struct {
	m   *manager.Manager
	src asynctask.Source
}

// New 构造中心门面。m 为 nil 返回 asynctask.ErrInvalidConfig。
//
// 注意：中心进程应调用 m.Start(ctx) 启动维护循环，否则「次数用尽 + 租约过期」的
// 远端任务会一直停在 running（远端 worker 已崩溃，没人会再回调）。
func New(m *manager.Manager) (*Center, error) {
	if m == nil {
		return nil, asynctask.ErrInvalidConfig
	}
	return &Center{m: m, src: m.Source()}, nil
}

// Fetch 代表远端 worker 认领一条任务。无任务返回 Available=false（不是错误）。
func (c *Center) Fetch(ctx context.Context, req wire.FetchRequest) (*wire.FetchResponse, error) {
	if strings.TrimSpace(req.TaskType) == "" || strings.TrimSpace(req.WorkerID) == "" {
		return nil, fmt.Errorf("%w: task_type and worker_id are required", asynctask.ErrInvalidConfig)
	}
	task, err := c.m.FetchFor(ctx, req.TaskType, req.WorkerID)
	if errors.Is(err, asynctask.ErrNoAsyncTask) {
		return &wire.FetchResponse{Available: false}, nil
	}
	if err != nil {
		return nil, err
	}
	return &wire.FetchResponse{Available: true, Task: wire.FromAsyncTask(task)}, nil
}

// Heartbeat 续租。非持有者或任务已终态返回 asynctask.ErrLeaseLost（远端据此取消执行）。
func (c *Center) Heartbeat(ctx context.Context, req wire.HeartbeatRequest) error {
	task, err := c.owned(ctx, req.TaskID, req.WorkerID)
	if err != nil {
		return err
	}
	return c.src.Renew(ctx, task)
}

// Callback 接收一次执行结果。
//
// 语义：
//   - 任务已是终态（重复回调/已被回收）→ 返回 nil（幂等 ACK），避免客户端无谓重试；
//   - 任务仍 running 但归属不是该 worker → ErrLeaseLost（客户端应记日志而非重试）；
//   - 归属匹配 → 走与本地形态相同的 Save（失权保护、重试决策、业务钩子都复用）。
func (c *Center) Callback(ctx context.Context, req wire.CallbackRequest) error {
	outcome, ok := wire.ParseOutcome(req.Status)
	if !ok {
		return fmt.Errorf("%w: unknown status %q", asynctask.ErrInvalidConfig, req.Status)
	}
	if strings.TrimSpace(req.TaskID) == "" || strings.TrimSpace(req.WorkerID) == "" {
		return fmt.Errorf("%w: task_id and worker_id are required", asynctask.ErrInvalidConfig)
	}
	task, err := c.m.Get(ctx, req.TaskID)
	if err != nil {
		return err
	}
	if task.Status != asynctask.StatusRunning {
		return nil // 已结算：幂等
	}
	if task.WorkerID != req.WorkerID {
		return asynctask.ErrLeaseLost
	}
	res := asynctask.Result{
		Outcome:     outcome,
		Failure:     asynctask.FailureKind(req.Failure),
		Payload:     req.Result,
		Duration:    time.Duration(req.DurationMS) * time.Millisecond,
		PauseReason: req.PauseReason,
	}
	if res.Duration < 0 {
		res.Duration = 0
	}
	if req.PauseUntilMS > 0 {
		res.PauseUntil = time.UnixMilli(req.PauseUntilMS)
	}
	if req.Error != "" {
		res.Err = errors.New(req.Error)
	}
	// 中心侧没有业务钩子（钩子在远端 worker 进程内已执行），callback 传 nil。
	return c.src.Save(ctx, task, res, nil)
}

// Release 主动放弃所有权。幂等：任务已终态/已清理/非该 worker 持有时都返回 nil。
func (c *Center) Release(ctx context.Context, req wire.ReleaseRequest) error {
	if strings.TrimSpace(req.TaskID) == "" {
		return fmt.Errorf("%w: task_id is required", asynctask.ErrInvalidConfig)
	}
	task, err := c.m.Get(ctx, req.TaskID)
	if errors.Is(err, asynctask.ErrAsyncTaskNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if task.Status != asynctask.StatusRunning || task.WorkerID != req.WorkerID {
		return nil
	}
	return c.src.Release(ctx, task)
}

// owned 读取任务并校验归属；非持有者/已终态返回 asynctask.ErrLeaseLost。
func (c *Center) owned(ctx context.Context, taskID, workerID string) (*asynctask.AsyncTask, error) {
	if strings.TrimSpace(taskID) == "" || strings.TrimSpace(workerID) == "" {
		return nil, fmt.Errorf("%w: task_id and worker_id are required", asynctask.ErrInvalidConfig)
	}
	task, err := c.m.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != asynctask.StatusRunning || task.WorkerID != workerID {
		return nil, asynctask.ErrLeaseLost
	}
	return task, nil
}
