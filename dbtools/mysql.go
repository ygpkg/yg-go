package dbtools

import (
	"fmt"
	"sync"

	"github.com/ygpkg/yg-go/logs"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	dbs       = map[string]*gorm.DB{}
	dbsLocker sync.RWMutex
)

type InitModelFunc func() error
type InitModelWithDBFunc func(db *gorm.DB) error

// InitModel 工具类，自动生成表结构，gorm中的AutoMIGRATE
func InitModel(db *gorm.DB, models ...interface{}) error {
	for _, v := range models {
		if err := db.AutoMigrate(v); err != nil {
			logs.Errorf("[init-db] auto migrate %T failed, %s", v, err)
			return err
		}
	}
	return nil
}

// DoInitModels 使用默认db初始化模型
func DoInitModels(imfs ...InitModelFunc) error {
	for _, imf := range imfs {
		if err := imf(); err != nil {
			logs.Errorf("[init-db] do %T failed, %s", imf, err)
			return err
		}
	}
	return nil
}

// DoInitModelsWithDB 使用db参数初始化模型
func DoInitModelsWithDB(db *gorm.DB, imfs ...InitModelWithDBFunc) error {
	for _, imf := range imfs {
		if err := imf(db); err != nil {
			logs.Errorf("[init-db] do %T failed, %s", imf, err)
			return err
		}
	}
	return nil
}

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

func InsertOrUpdate(db *gorm.DB, v interface{}, columns ...string) error {
	return db.Clauses(clause.OnConflict{
		DoUpdates: clause.AssignmentColumns(columns),
	}).Create(v).Error
}

// RegistryDB 注册数据库
func RegistryDB(name string, db *gorm.DB) {
	dbsLocker.Lock()
	defer dbsLocker.Unlock()
	if _, ok := dbs[name]; ok {
		logs.Errorf("[init-db] db %s is already exist", name)
		return
	}
	dbs[name] = db
	logs.Infof("[init-db] registry db %s success", name)
}

// DB 获取数据库连接
func DB(name string) *gorm.DB {
	dbsLocker.RLock()
	db, ok := dbs[name]
	dbsLocker.RUnlock()
	if !ok {
		panic(fmt.Errorf("db %s is nil", name))
	}
	return db
}

// DBExists 判断数据库是否存在
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
