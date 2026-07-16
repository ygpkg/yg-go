package v2

import (
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type InitModelFunc func() error
type InitModelWithDBFunc func(db *gorm.DB) error

func InitModel(db *gorm.DB, models ...interface{}) error {
	for _, v := range models {
		if err := db.AutoMigrate(v); err != nil {
			logs.Errorf("[dbv2] auto migrate %T failed: %s", v, err)
			return err
		}
	}
	return nil
}

func DoInitModels(imfs ...InitModelFunc) error {
	for _, imf := range imfs {
		if err := imf(); err != nil {
			logs.Errorf("[dbv2] do %T failed: %s", imf, err)
			return err
		}
	}
	return nil
}

func DoInitModelsWithDB(db *gorm.DB, imfs ...InitModelWithDBFunc) error {
	for _, imf := range imfs {
		if err := imf(db); err != nil {
			logs.Errorf("[dbv2] do %T failed: %s", imf, err)
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
