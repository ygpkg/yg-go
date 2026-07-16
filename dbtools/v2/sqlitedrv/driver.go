package sqlitedrv

import (
	dbv2 "github.com/ygpkg/yg-go/dbtools/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	dbv2.Register("sqlite", func(dsn string) (gorm.Dialector, error) {
		return sqlite.Open(dsn), nil
	})
	dbv2.Register("sqlite3", func(dsn string) (gorm.Dialector, error) {
		return sqlite.Open(dsn), nil
	})
}
