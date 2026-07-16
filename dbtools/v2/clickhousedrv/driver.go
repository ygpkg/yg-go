package clickhousedrv

import (
	dbv2 "github.com/ygpkg/yg-go/dbtools/v2"
	"gorm.io/driver/clickhouse"
	"gorm.io/gorm"
)

func init() {
	dbv2.Register("clickhouse", func(dsn string) (gorm.Dialector, error) {
		return clickhouse.Open(dsn), nil
	})
}
