package clickhousetool

import (
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

// ConnectClickHouse 初始化ClickHouse数据库
func ConnectClickHouse(name, dburl string) (*gorm.DB, error) {
	db, err := gorm.Open(clickhouse.Open(dburl), &gorm.Config{
		CreateBatchSize: 200,
	})
	if err != nil {
		logs.Errorf("[init-db] open clickhouse(%s) failed, %s", name, err)
		return nil, err
	}
	logs.Infof("[init-db] open clickhouse(%s) success", name)

	if name == "" {
		name = "default"
	}

	return db, nil
}
