package sqlitetool

import (
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// InitSQLite 初始化sqlite数据库
func InitSQLite(name, dburl string) (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(dburl), &gorm.Config{
		CreateBatchSize: 20,
	})
	if err != nil {
		logs.Errorf("[init-db] open sqlite(%s) failed, %s", name, err)
		return nil, err
	}
	logs.Infof("[init-db] open sqlite(%s) success", name)

	if name == "" {
		name = "default"
	}

	return db, nil
}
