package db

import "github.com/google/uuid"

// genUUID 生成 UUID v4，用于任务与尝试记录的 id（契约要求 varchar(36) UUID v4）。
//
// 与 manager/id.go 中的同名函数各自独立：两包都只依赖 uuid，无需共享。
func genUUID() string { return uuid.NewString() }
