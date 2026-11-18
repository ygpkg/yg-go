package settings

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/cache"
	dbtools "github.com/ygpkg/yg-go/dbtools/v2"
	"github.com/ygpkg/yg-go/logs"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TableNameSettings = "core_settings"
)

const (
	SettingGroupCore = "core"
)

type ValueType string

const (
	ValueSecret   ValueType = "secret"
	ValuePassword ValueType = "password"
	ValueText     ValueType = "text"
	ValueInt64    ValueType = "int64"
	ValueBool     ValueType = "bool"
	ValueJSON     ValueType = "json"
	ValueYaml     ValueType = "yaml"
)

// SettingItem 系统配置
type SettingItem struct {
	gorm.Model

	Group     string    `json:"group" yaml:"group" gorm:"column:group;type:varchar(16);uniqueIndex:idx_setting,priority:2,unique"`
	Key       string    `json:"key" yaml:"key" gorm:"column:key;type:varchar(64);uniqueIndex:idx_setting,priority:3,unique"`
	Name      string    `json:"name" yaml:"name" gorm:"column:name;type:varchar(64)"`
	Describe  string    `json:"describe" yaml:"describe" gorm:"column:describe;type:varchar(128)"`
	ValueType ValueType `json:"value_type" yaml:"value_type" gorm:"column:value_type;default:text;type:varchar(16)"`
	Value     string    `json:"value" yaml:"value" gorm:"column:value;type:text"`
	Default   string    `json:"default" yaml:"default" gorm:"column:default;type:text"`
}

// TableName .
func (*SettingItem) TableName() string { return TableNameSettings }

// BeforeCreate .
func (item *SettingItem) BeforeSave(tx *gorm.DB) error {
	if item.ValueType == ValueSecret || item.ValueType == ValuePassword {
		item.Value = EncryptSecret(item.Value)
	}
	return nil
}

// Identify 唯一建
func (item SettingItem) Identify() string {
	return fmt.Sprintf("%s/%s", item.Group, item.Key)
}

// SecretValue 密码原文
func (item *SettingItem) SecretValue() string {
	if item.ValueType == ValueSecret || item.ValueType == ValuePassword {
		return DecryptSecret(item.Value)
	}
	return item.Value
}

// dbWithContext 返回携带 ctx 的数据库句柄，使 gorm 日志能够带出 reqid 等上下文字段。
func dbWithContext(ctx context.Context) *gorm.DB {
	return dbtools.Core().WithContext(ctx)
}

// InitDB . 初始化配置项表结构。初始化路径无追踪上下文，使用 context.Background()。
func InitDB() error {
	return dbtools.InitModel(dbtools.Core(), &SettingItem{})
}

// GetByID .
func GetByID(ctx context.Context, id uint) (*SettingItem, error) {
	db := dbWithContext(ctx)
	ret := &SettingItem{}
	err := db.Table(TableNameSettings).
		Where("id = ?", id).
		Find(ret).Error
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Set .
func Set(ctx context.Context, group, key, value string) error {
	return SetText(ctx, group, key, value)
}

func SetText(ctx context.Context, group, key, value string) error {
	si := &SettingItem{
		Group:     group,
		Key:       key,
		Value:     value,
		ValueType: ValueText,
	}
	return UpsertSetting(ctx, si)
}

// SetYaml 插入yaml配置
func SetYaml(ctx context.Context, group, key string, value interface{}) error {
	vData, err := yaml.Marshal(value)
	if err != nil {
		logs.ErrorContextf(ctx, "[settings] yaml marshal failed, %s", err)
		return err
	}
	si := &SettingItem{
		Group:     group,
		Key:       key,
		Value:     string(vData),
		ValueType: ValueYaml,
	}
	return UpsertSetting(ctx, si)
}

// UpsertSetting or update the trade calendar of a stock.
func UpsertSetting(ctx context.Context, v *SettingItem) error {
	db := dbWithContext(ctx)
	rdsKey := redisCacheKey(v.Group, v.Key)
	if err := cache.Std().Delete(rdsKey); err != nil {
		logs.WarnContextf(ctx, "[settings] delete cache %s failed, %s", rdsKey, err)
	}
	err := db.Table(TableNameSettings).
		Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{"name", "describe", "value", "value_type", "default"}),
		}).Create(v).Error
	if err != nil {
		logs.ErrorContextf(ctx, "[settings] upsert %s failed, %s", v.Identify(), err)
		return err
	}
	return nil
}

// Get 获取数据库或者缓存配置项
func Get(ctx context.Context, group, key string) (*SettingItem, error) {
	db := dbWithContext(ctx)
	ret := &SettingItem{}

	rdsKey := redisCacheKey(group, key)
	err := cache.Std().Get(rdsKey, ret)
	if err == nil && ret.Value != "" {
		return ret, nil
	}

	err = db.Table(TableNameSettings).
		Where(map[string]interface{}{"group": group, "key": key}).
		First(ret).Error
	if err != nil {
		logs.ErrorContextf(ctx, "[settings] get %s failed, %s", group+"/"+key, err)
		return nil, err
	}

	if ret.Value != "" {
		if err := cache.Std().Set(rdsKey, ret, time.Minute*5); err != nil {
			logs.WarnContextf(ctx, "[settings] set cache %s failed, %s", rdsKey, err)
		}
	}

	return ret, nil
}

// GetValue 获取配置值，如果是加密配置，返回解密后的值
func GetValue(ctx context.Context, group, key string) (string, error) {
	si, err := Get(ctx, group, key)
	if err != nil {
		return "", err
	}
	return si.SecretValue(), nil
}

// GetYaml 获取yaml配置
func GetYaml(ctx context.Context, group, key string, value interface{}) error {
	text, err := GetValue(ctx, group, key)
	if err != nil {
		return err
	}
	return yaml.Unmarshal([]byte(text), value)
}

// GetText 获取文本配置
func GetText(ctx context.Context, group, key string) (string, error) {
	return GetValue(ctx, group, key)
}

// GetSecret 获取密码配置
func GetSecret(ctx context.Context, group, key string) (string, error) {
	si, err := Get(ctx, group, key)
	if err != nil {
		return "", err
	}
	return DecryptSecret(si.Value), nil
}

// GetSecretYaml 获取密码配置YAML
func GetSecretYaml(ctx context.Context, group, key string, value interface{}) error {
	date, err := GetSecret(ctx, group, key)
	if err != nil {
		return err
	}
	return yaml.Unmarshal([]byte(date), value)
}

// GetJSON 获取json配置
func GetJSON(ctx context.Context, group, key string, value interface{}) error {
	text, err := GetValue(ctx, group, key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(text), value)
}

// List 配置列表
func List(ctx context.Context, group string, keys ...string) ([]*SettingItem, error) {
	db := dbWithContext(ctx)
	ret := []*SettingItem{}
	err := db.Table(TableNameSettings).
		Where(map[string]interface{}{"group": group, "key": keys}).
		Find(&ret).Error
	if err != nil {
		logs.ErrorContextf(ctx, "[settings] list %s failed, %s", group, err)
		return nil, err
	}
	return ret, nil
}

// Updates 更新settings值
func Updates(ctx context.Context, sets ...*SettingItem) error {
	db := dbWithContext(ctx)
	for _, set := range sets {
		sql := db.Table(TableNameSettings)
		if set.ID != 0 {
			sql = sql.Where("id = ?", set.ID)
		} else {
			sql = sql.Where(map[string]interface{}{"group": set.Group, "key": set.Key})
		}
		update := &SettingItem{
			Value:     set.Value,
			ValueType: set.ValueType,
			Describe:  set.Describe,
			Name:      set.Name,
		}
		err := sql.Select("value", "value_type", "describe", "name").Updates(update).Error
		if err != nil {
			logs.ErrorContextf(ctx, "[settings] update %s failed, %s", set.Identify(), err)
			return err
		}
		rdsKey := redisCacheKey(set.Group, set.Key)
		if err := cache.Std().Delete(rdsKey); err != nil {
			logs.WarnContextf(ctx, "[settings] delete cache %s failed, %s", rdsKey, err)
		}
	}
	return nil
}

func redisCacheKey(group, key string) string {
	return fmt.Sprintf("core_setting::%s::%s", group, key)
}
