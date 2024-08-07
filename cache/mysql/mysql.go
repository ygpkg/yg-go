package mysql

import (
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/cache/cachetype"
	"github.com/ygpkg/yg-go/logs"
	"gorm.io/gorm"
)

const (
	TableName = "sys_cache"
)

type CacheVal struct {
	Key       string    `gorm:"column:id;type:varchar(64);primaryKey"`
	Val       string    `gorm:"column:value;type:varchar(1024)"`
	ExpiredAt time.Time `gorm:"column:expired_at;type:timestamp"`
	Version   uint16    `gorm:"column:version"`
}

func (*CacheVal) TableName() string { return TableName }

var _ cachetype.Cache = (*MysqlCache)(nil)

type MysqlCache struct {
	db *gorm.DB
}

func NewCache(db *gorm.DB) *MysqlCache {
	mc := &MysqlCache{db}
	if err := db.AutoMigrate(&CacheVal{}); err != nil {
		logs.Errorf("[mysql-cache] migrate table failed, %s", err)
		panic(fmt.Errorf("migrate cache table failed, %s", err))
	}
	return mc
}

func (mc *MysqlCache) Get(key string, val interface{}) error {
	cv, err := getCacheVal(mc.db, key)
	if err != nil {
		return err
	}

	if time.Now().Before(cv.ExpiredAt) {
		logs.Debugf("[mysql-cache] got key(%s) expired at %s", key, cv.ExpiredAt)
		return fmt.Errorf("key(%s) expired at %s", key, cv.ExpiredAt)
	}

	if err := cachetype.Unmarshal([]byte(cv.Val), val); err != nil {
		logs.Errorf("[mysql-cache] got key(%s) unmarshal failed %s", key, err)
		return err
	}

	return nil
}

func (mc *MysqlCache) Set(key string, val interface{}, timeout time.Duration) error {
	cv, err := getCacheVal(mc.db, key)
	if err != nil {
		return mc.db.Create(&CacheVal{
			Key:       key,
			Val:       cachetype.Marshal(val),
			ExpiredAt: time.Now().Add(timeout),
		}).Error
	}
	logs.Infof("got cv: %v", cv)
	return mc.db.Table(TableName).
		Where("id = ? AND version = ?", cv.Key, cv.Version).Updates(map[string]interface{}{
		"expired_at": time.Now().Add(timeout),
		"value":      cachetype.Marshal(val),
		"version":    gorm.Expr("version+1"),
	}).Error
}

func (mc *MysqlCache) IsExist(key string) bool {
	var count int64
	err := mc.db.Table(TableName).
		Where("id = ? AND expired_at > ?", key, time.Now()).
		Count(&count).Error
	if err != nil {
		logs.Warnf("[mysql-cache] exists key(%s) failed, %s", key, err)
		return false
	}
	return count > 0
}

func (mc *MysqlCache) Delete(key string) error {
	err := mc.db.Table(TableName).
		Where("id = ?", key).
		Unscoped().
		Delete(nil).Error
	if err != nil {
		logs.Warnf("[mysql-cache] delete key(%s) failed, %s", key, err)
		return err
	}
	return nil
}

func getCacheVal(db *gorm.DB, key string) (*CacheVal, error) {
	cv := &CacheVal{}
	err := db.Table(TableName).
		Where("id = ?", key).
		Find(cv).Error
	if err != nil {
		logs.Warnf("[mysql-cache] got key(%s) failed, %s", key, err)
		return nil, err
	}
	if cv.Key == "" {
		return nil, gorm.ErrRecordNotFound
	}
	return cv, nil
}
