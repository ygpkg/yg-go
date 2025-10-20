package dbtools

import (
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func InitMySQL(name, dburl string) (*gorm.DB, error) {
	db, err := gorm.Open(mysql.Open(dburl), &gorm.Config{
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
	dbsLocker.Lock()
	dbs[name] = db
	dbsLocker.Unlock()

	return db, nil
}

// InitMutilMySQL 批量初始化mysql数据库
func InitMutilMySQL(dburls map[string]string) error {
	for name, dburl := range dburls {
		logs.Infof("[init-db] init mysql(%s) %s", name, dburl)
		db, err := InitMySQL(name, dburl)
		if err != nil {
			return err
		}
		db.Logger = logs.GetGorm("gorm")
	}
	return nil
}
