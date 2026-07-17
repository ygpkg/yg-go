package pgdrv

import (
	dbtools "github.com/ygpkg/yg-go/dbtools/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func init() {
	dbtools.Register("postgres", func(dsn string) (gorm.Dialector, error) {
		return postgres.Open(dsn), nil
	})
	dbtools.Register("pg", func(dsn string) (gorm.Dialector, error) {
		return postgres.Open(dsn), nil
	})
	dbtools.Register("postgresql", func(dsn string) (gorm.Dialector, error) {
		return postgres.Open(dsn), nil
	})
}
