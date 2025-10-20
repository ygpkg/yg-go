package dbtools

import (
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InitModelFunc func() error
type InitModelWithDBFunc func(db *gorm.DB) error

// InitModel 工具类，自动生成表结构，gorm中的AutoMIGRATE
func InitModel(db *gorm.DB, models ...interface{}) error {
	for _, v := range models {
		if err := db.AutoMigrate(v); err != nil {
			logs.Errorf("[init-db] auto migrate %T failed, %s", v, err)
			return err
		}
	}
	return nil
}

// DoInitModels 使用默认db初始化模型
func DoInitModels(imfs ...InitModelFunc) error {
	for _, imf := range imfs {
		if err := imf(); err != nil {
			logs.Errorf("[init-db] do %T failed, %s", imf, err)
			return err
		}
	}
	return nil
}

// DoInitModelsWithDB 使用db参数初始化模型
func DoInitModelsWithDB(db *gorm.DB, imfs ...InitModelWithDBFunc) error {
	for _, imf := range imfs {
		if err := imf(db); err != nil {
			logs.Errorf("[init-db] do %T failed, %s", imf, err)
			return err
		}
	}
	return nil
}

func InsertOrUpdate(db *gorm.DB, v interface{}, columns ...string) error {
	return db.Clauses(clause.OnConflict{
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(v).Error
}
