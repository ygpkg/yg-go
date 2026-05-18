package pgtool

import (
	"time"

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
	sqlDB, err := db.DB()
	if err != nil {
		logs.Errorf("[init-db] get sqlDB from mysql(%s) failed, %s", name, err)
		return nil, err
	}

	// 设置连接池参数
	sqlDB.SetMaxOpenConns(50)                  // 最大打开连接数
	sqlDB.SetMaxIdleConns(10)                  // 最大空闲连接数
	sqlDB.SetConnMaxLifetime(30 * time.Minute) // 连接最大存活时间
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)  // 空闲连接最大存活时间

	if name == "" {
		name = "default"
	}

	return db, nil
}
