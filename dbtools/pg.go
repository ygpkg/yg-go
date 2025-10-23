package dbtools

import (
	"github.com/ygpkg/yg-go/dbtools/pgtool"
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

// InitMutilPostgres 批量初始化postgres数据库
func InitMutilPostgres(dburls map[string]string) error {
	for name, dburl := range dburls {
		logs.Infof("[init-db] init postgres(%s) %s", name, dburl)
		db, err := InitPostgres(name, dburl)
		if err != nil {
			return err
		}
		db.Logger = logs.GetGorm("gorm")
	}
	return nil
}

// InitPostgres 初始化
func InitPostgres(name, postgres string) (*gorm.DB, error) {
	db, err := pgtool.ConnectPostgres(name, postgres)
	if err != nil {
		return nil, err
	}
	RegistryDB(name, db)
	return db, nil
}
