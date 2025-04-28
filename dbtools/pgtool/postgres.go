package pgtool

import (
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// ConnectPostgres 初始化Postgres数据库
func ConnectPostgres(name, dburl string) (*gorm.DB, error) {
	db, err := gorm.Open(postgres.Open(dburl), &gorm.Config{
		CreateBatchSize: 200,
	})
	if err != nil {
		logs.Errorf("[init-db] open mysql(%s) failed, %s", name, err)
		return nil, err
	}
	logs.Infof("[init-db] open mysql(%s) success", name)

	if name == "" {
		name = "default"
	}

	return db, nil
}
