package prompt

import (
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/prompt/model"
	"gorm.io/gorm"
)

// InitDB initializes the core_prompt and core_prompt_version tables using the default database connection.
// It is designed to be used with dbtools.DoInitModels for automatic migration on startup.
func InitDB() error {
	return dbtools.InitModel(dbtools.Core(),
		&model.CorePrompt{},
		&model.CorePromptVersion{},
	)
}

// InitModel initializes the core_prompt and core_prompt_version tables with the given database connection.
// Use this when you need to specify a custom database connection instead of the default.
func InitModel(db *gorm.DB) error {
	return dbtools.InitModel(db,
		&model.CorePrompt{},
		&model.CorePromptVersion{},
	)
}
