package gormdao

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// StringID 仅提供 string 主键及插入前的自动生成逻辑，
// 供不需要软删除/时间戳的内置表实体内嵌（如硬删除的执行记录表）。
type StringID struct {
	ID string `gorm:"column:id;primaryKey;type:varchar(36)"`
}

// BeforeCreate 在插入前自动生成主键 ID（UUID v7，时间有序，主键索引友好）。
// 内嵌 StringID / BaseEntity 的实体自动继承该钩子；已显式赋值的 ID 不会被覆盖。
func (s *StringID) BeforeCreate(_ *gorm.DB) error {
	if s.ID == "" {
		s.ID = uuid.Must(uuid.NewV7()).String()
	}
	return nil
}

// BaseEntity 内置表通用基类：语义等同 gorm.Model，但主键为 string（varchar(36)）。
// 内嵌该结构可获得 ID/CreatedAt/UpdatedAt/DeletedAt 四个列，并自动生成主键 ID。
type BaseEntity struct {
	StringID
	CreatedAt time.Time      `gorm:"column:created_at"`
	UpdatedAt time.Time      `gorm:"column:updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"column:deleted_at;index"`
}
