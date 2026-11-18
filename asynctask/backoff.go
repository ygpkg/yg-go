package asynctask

import "time"

// Backoff 计算第 attempt 次失败后的重试退避时长（attempt 从 1 开始，指第 1 次失败后等待的时长）。
//
// 本类型下沉到契约内核，使 source/db（DB Source）与 manager（管理面）都能引用它，
// 而不产生包间循环依赖。具体退避策略（如 manager.DefaultBackoff 的指数+抖动）由调用方注入。
type Backoff func(attempt int) time.Duration
