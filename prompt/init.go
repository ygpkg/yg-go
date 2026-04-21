package prompt

import (
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/prompt/model"
	"gorm.io/gorm"
)

// InitModel 初始化 core_prompt 与 core_prompt_version 表的 AutoMigrate
func InitModel(db *gorm.DB) error {
	return dbtools.InitModel(db,
		&model.CorePrompt{},
		&model.CorePromptVersion{},
	)
}
