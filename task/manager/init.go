package manager

import (
	"github.com/ygpkg/yg-go/dbtools"
	"github.com/ygpkg/yg-go/task/model"
	"gorm.io/gorm"
)

// Init 初始化处理
func Init(db *gorm.DB) error {
	if err := dbtools.InitModel(db, &model.TaskEntity{}); err != nil {
		return err
	}
	return nil
}
