package manager

import "time"

// 时间单位与时钟的小别名，便于在 manager 内部统一引用（也方便测试替换时钟）。
const (
	timeMillisecond = time.Millisecond
)

// timeDuration 别名，用于返回类型（保持可读）。
type timeDuration = time.Duration

// timeNow 返回应用时钟当前时间（用于 Submit 写 created_at 等非调度列）。
// 调度相关时间（next_run_at/lease_until 的推进）一律走 DB 表达式，不用这里。
func timeNow() time.Time { return time.Now() }
