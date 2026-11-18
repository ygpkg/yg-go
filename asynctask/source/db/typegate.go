package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// 任务类型开关（type gate）的存储方法。
//
// 语义：core_async_task_type 无该类型行 = 启用（fail-open）。Disable 写 enabled=false，
// Enable 写 enabled=true。IsTypeEnabled 查不到行或查询失败时返回 true（fail-open）。

// IsTypeEnabled 查询某类型是否启用；无行或查询失败返回 true（fail-open）。
func (s *store) IsTypeEnabled(ctx context.Context, taskType string) bool {
	var row CoreAsyncTaskType
	err := s.db.WithContext(ctx).Where("task_type = ?", taskType).Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return true
	}
	if err != nil {
		return true // fail-open：查询失败不误停任务
	}
	return row.Enabled
}

// SetTypeEnabled 写入/更新某类型的启用状态（enabled=true/false），幂等。
func (s *store) SetTypeEnabled(ctx context.Context, taskType string, enabled bool) error {
	// 用 map 显式写入，避免 GORM 对 bool 零值(false)套用列默认 true。
	row := map[string]any{"id": genUUID(), "task_type": taskType, "enabled": enabled, "updated_at": time.Now()}
	return s.db.WithContext(ctx).Table(CoreAsyncTaskTypeTableName).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "task_type"}},
			DoUpdates: clause.AssignmentColumns([]string{"enabled", "updated_at"}),
		}).
		Create(row).Error
}
