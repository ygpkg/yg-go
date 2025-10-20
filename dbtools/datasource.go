package dbtools

import (
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/ygpkg/yg-go/logs"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

var (
	dbs       = map[string]*gorm.DB{}
	dbsLocker sync.RWMutex
)

func InitDBConn(name, dburl string) (*gorm.DB, error) {
	dsn, err := url.Parse(dburl)
	if err != nil {
		logs.Errorf("[init-db] parse dburl(%s) failed, %s", dburl, err)
		return nil, err
	}
	var db *gorm.DB
	switch dsn.Scheme {
	case "mysql":
		uri, err := NormalizeMySQL(dsn)
		if err != nil {
			logs.Errorf("[init-db] normalize mysql(%s) failed, %s", dburl, err)
			return nil, err
		}
		db, err = gorm.Open(mysql.Open(uri), &gorm.Config{
			CreateBatchSize: 200,
		})
	case "sqlite", "sqlite3":
		dbPath := strings.TrimPrefix(dsn.Path, "/")
		if dbPath == "" {
			dbPath = ":memory:"
		}

		db, err = gorm.Open(sqlite.Open(dbPath), &gorm.Config{
			CreateBatchSize: 200,
		})
	case "postgresql", "postgres", "pg":
		db, err = gorm.Open(postgres.Open(dburl), &gorm.Config{
			CreateBatchSize: 200,
		})
	default:
		logs.Errorf("[init-db] unsupported db scheme %s", dsn.Scheme)
		return nil, fmt.Errorf("unsupported db scheme %s", dsn.Scheme)
	}
	if err != nil {
		logs.Errorf("[init-db] open %s(%s) failed, %s", dsn.Scheme, name, err)
		return nil, err
	}
	logs.Infof("[init-db] open %s(%s) success", dsn.Scheme, name)

	if name == "" {
		name = "default"
	}
	dbsLocker.Lock()
	dbs[name] = db
	dbsLocker.Unlock()

	return db, nil
}

// InitMutilDBConn 批量初始化数据库
func InitMutilDBConn(dburls map[string]string) error {
	for name, dburl := range dburls {
		logs.Infof("[init-db] init db(%s) %s", name, dburl)
		db, err := InitDBConn(name, dburl)
		if err != nil {
			return err
		}
		db.Logger = logs.GetGorm("gorm")
	}
	return nil
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

// NormalizeMySQL 将 mysql:// URI 转换为 go-sql-driver/mysql 的 DSN
func NormalizeMySQL(u *url.URL) (string, error) {
	user := u.User.Username()
	pass, _ := u.User.Password()
	host := u.Hostname()
	port := u.Port()
	if port == "" {
		port = "3306"
	}
	db := strings.TrimPrefix(u.Path, "/")

	query := u.RawQuery
	if query != "" {
		query = "?" + query
	}

	return fmt.Sprintf("%s:%s@tcp(%s:%s)/%s%s", user, pass, host, port, db, query), nil
}
