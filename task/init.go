package task

import (
	"github.com/ygpkg/yg-go/dbtools"
	"gorm.io/gorm"
)

// Init 初始化处理
func Init(db *gorm.DB) error {
	if err := dbtools.InitModel(db, &TaskEntity{}); err != nil {
		return err
	}
	return nil
}
