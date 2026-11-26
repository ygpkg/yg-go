package asynctask

import (
	"context"
	"encoding/json"
)

// 取消语义（设计 §9.1 / Y1–Y2）：Cancel 在**状态翻转的同一事务内**发出 PG 通知，
// 执行侧的常驻监听器收到后按「本地活跃执行表」精确匹配并取消本次执行。
//
// 为什么不是「靠续租发现取消」：续租周期是 LeaseTTL/3（默认 20s），取消要等一个周期才生效，
// 与「感知 p99 ≤2s」差一个数量级。通知只负责**快**，续租与定时补查负责**正确**：
// 通知丢失、监听重连、执行登记与取消事务撞车这三种情况，最终都由「续租失权 / 补查发现终态」兜住。

// CancelSignal 是一条取消通知的载荷（Cancel 事务内 pg_notify 的 body）。
//
// 带 generation + attempt 是为了**晚到取消不影响新执行**：取消只对「当时那次执行」生效，
// 执行侧按 (TaskID, Generation, Attempt) 精确匹配本地活跃执行表；
// 若用户取消后又重放/重新提交（generation 变了、或 attempt 变了），晚到的通知匹配不到任何执行，
// 于是一个字都不改——旧通知不会杀掉新执行。
type CancelSignal struct {
	TaskID string `json:"task_id"`
	// Generation / Attempt 为 0 表示通配（只按 TaskID 匹配）：
	// Cancel 时任务还在 pending（没有活跃执行）就是这种形态，执行侧不会命中任何在途执行。
	Generation int `json:"generation,omitempty"`
	Attempt    int `json:"attempt,omitempty"`
}

// Encode 把取消信号序列化为通知载荷（非法结构不会出现，故出错时返回空串）。
func (s CancelSignal) Encode() string {
	b, err := json.Marshal(s)
	if err != nil {
		return ""
	}
	return string(b)
}

// DecodeCancelSignal 解析通知载荷；载荷非法（空串、非 JSON、缺 task_id）返回 ok=false。
//
// 监听器对非法载荷只记日志并丢弃：通知不参与正确性，兜底补查会接手。
func DecodeCancelSignal(payload string) (sig CancelSignal, ok bool) {
	if payload == "" {
		return CancelSignal{}, false
	}
	if err := json.Unmarshal([]byte(payload), &sig); err != nil {
		return CancelSignal{}, false
	}
	if sig.TaskID == "" {
		return CancelSignal{}, false
	}
	return sig, true
}

// Match 判断本信号是否命中一次具体执行（taskID + generation + attempt）。
//
// 规则：TaskID 必须相等；Generation/Attempt 为 0 视作通配（不限定）。
// 只按 id 匹配会误杀「同一任务被重放后的新执行」，所以 cancel 侧一定带上 generation+attempt。
func (s CancelSignal) Match(taskID string, generation, attempt int) bool {
	if s.TaskID == "" || s.TaskID != taskID {
		return false
	}
	if s.Generation != 0 && s.Generation != generation {
		return false
	}
	if s.Attempt != 0 && s.Attempt != attempt {
		return false
	}
	return true
}

// CancelWatcher 是执行侧的取消通知来源（可选）。实现负责「常驻监听 + 断线重连」，
// Worker 负责「收到信号 → 查本地活跃执行表 → 取消匹配的那次执行」。
//
// 依赖边界：接口在根包（worker 只依赖根包），具体实现（PG LISTEN 等）在独立子包，
// 例如 asynctask/cancelwatch 的 PGWatcher。实现不承载正确性：
// 丢通知、重连空窗都由 Worker 的定时补查与续租失权兜住。
type CancelWatcher interface {
	// Watch 返回取消信号通道。ctx 是监听的生命周期：ctx 取消时实现应停止监听。
	//
	// 实现**不保证关闭通道**：监听协程可能正阻塞在等待通知上（网络黑洞时 socket
	// 迟迟不返回），若关闭通道，投递方就会撞上 send on closed channel（进程级 panic）。
	// 因此消费方必须靠自己的 ctx 退出消费循环，不能依赖「通道关闭 = 监听结束」；
	// 实现内部重连期间同样不关闭通道。首次调用返回错误表示未能启动监听，
	// 调用方可等一会儿重试 Watch（实现不应在后台静默重试配置错误）。
	Watch(ctx context.Context) (<-chan CancelSignal, error)
}
