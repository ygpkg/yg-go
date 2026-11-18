package manager

import "github.com/google/uuid"

// genUUID 生成 UUID v4，用于任务与尝试记录的 id（契约要求 varchar(36) UUID v4）。
func genUUID() string { return uuid.NewString() }
