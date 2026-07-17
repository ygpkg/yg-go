package dbtools

import (
	"fmt"
	"sync"
	"time"

	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

var (
	dbs       = map[string]*gorm.DB{}
	dbsLocker sync.RWMutex
)

func InitDBConn(name, dburl string) (*gorm.DB, error) {
	db, err := Open(dburl)
	if err != nil {
		return nil, err
	}
	db.Logger = logs.GetGorm("gorm")
	if name == "" {
		name = "default"
	}
	dbsLocker.Lock()
	dbs[name] = db
	dbsLocker.Unlock()
	return db, nil
}

func InitMultiDBConn(dburls map[string]string) error {
	for name, dburl := range dburls {
		logs.Infof("[dbv2] init db(%s) %s", name, dburl)
		if _, err := InitDBConn(name, dburl); err != nil {
			return err
		}
	}
	return nil
}

func RegistryDB(name string, db *gorm.DB) {
	dbsLocker.Lock()
	defer dbsLocker.Unlock()
	if _, ok := dbs[name]; ok {
		logs.Errorf("[dbv2] db %s already exists", name)
		return
	}
	dbs[name] = db
	logs.Infof("[dbv2] registry db %s success", name)
}

func DB(name string) *gorm.DB {
	dbsLocker.RLock()
	db, ok := dbs[name]
	dbsLocker.RUnlock()
	if !ok {
		panic(fmt.Errorf("db %s is nil", name))
	}
	return db
}

func DBExists(name string) bool {
	dbsLocker.RLock()
	defer dbsLocker.RUnlock()
	_, ok := dbs[name]
	return ok
}

func Std() *gorm.DB {
	return DB("default")
}

func Core() *gorm.DB {
	return DB("core")
}

func Account() *gorm.DB {
	return DB("account")
}

func InitPostgresWithPool(name, dburl string) (*gorm.DB, error) {
	db, err := InitDBConn(name, dburl)
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetMaxIdleConns(10)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)
	sqlDB.SetConnMaxIdleTime(5 * time.Minute)
	return db, nil
}
