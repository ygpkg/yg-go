package pgdrv

import (
	dbv2 "github.com/ygpkg/yg-go/dbtools/v2"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func init() {
	dbv2.Register("postgres", func(dsn string) (gorm.Dialector, error) {
		return postgres.Open(dsn), nil
	})
	dbv2.Register("pg", func(dsn string) (gorm.Dialector, error) {
		return postgres.Open(dsn), nil
	})
	dbv2.Register("postgresql", func(dsn string) (gorm.Dialector, error) {
		return postgres.Open(dsn), nil
	})
}
