package sqlitedrv

import (
	dbtools "github.com/ygpkg/yg-go/dbtools/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func init() {
	dbtools.Register("sqlite", func(dsn string) (gorm.Dialector, error) {
		return sqlite.Open(dsn), nil
	})
	dbtools.Register("sqlite3", func(dsn string) (gorm.Dialector, error) {
		return sqlite.Open(dsn), nil
	})
}
