package settings

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/ygpkg/yg-go/cache"
	"github.com/ygpkg/yg-go/dbtools"
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

	Group     string    `json:"group" gorm:"column:group;type:varchar(16);uniqueIndex:idx_setting,priority:2,unique"`
	Key       string    `json:"key" gorm:"column:key;type:varchar(64);uniqueIndex:idx_setting,priority:3,unique"`
	Name      string    `json:"name" gorm:"column:name;type:varchar(64)"`
	Describe  string    `json:"describe" gorm:"column:describe;type:varchar(128)"`
	ValueType ValueType `json:"value_type" gorm:"column:value_type;default:text;type:varchar(16)"`
	Value     string    `json:"value" gorm:"column:value;type:text"`
	Default   string    `json:"default" gorm:"column:default;type:text"`
}

// TableName .
func (*SettingItem) TableName() string { return TableNameSettings }

// BeforeCreate .
func (item *SettingItem) BeforeCreate(tx *gorm.DB) error {
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

// InitDB .
func InitDB() error {
	return dbtools.InitModel(dbtools.Core(), &SettingItem{})
}

// GetByID .
func GetByID(id uint) (*SettingItem, error) {
	ret := &SettingItem{}
	err := dbtools.Core().Table(TableNameSettings).
		Where("id = ?", id).
		Find(ret).Error
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Set .
func Set(group, key, value string) error {
	return SetText(group, key, value)
}

func SetText(group, key, value string) error {
	si := &SettingItem{
		Group:     group,
		Key:       key,
		Value:     value,
		ValueType: ValueText,
	}
	return UpsertSetting(si)
}

// SetYaml 插入yaml配置
func SetYaml(group, key string, value interface{}) error {
	vData, err := yaml.Marshal(value)
	if err != nil {
		logs.Errorf("[settings] yaml marshal failed, %s", err)
		return err
	}
	si := &SettingItem{
		Group:     group,
		Key:       key,
		Value:     string(vData),
		ValueType: ValueYaml,
	}
	return UpsertSetting(si)
}

// UpsertSetting or update the trade calendar of a stock.
func UpsertSetting(v *SettingItem) error {
	rdsKey := redisCacheKey(v.Group, v.Key)
	cache.Std().Delete(rdsKey)
	return dbtools.Core().Table(TableNameSettings).
		Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{"name", "describe", "value", "value_type", "default"}),
		}).Create(v).Error
}

// Get 获取数据库或者缓存配置项
func Get(group, key string) (*SettingItem, error) {
	ret := &SettingItem{}

	rdsKey := redisCacheKey(group, key)
	err := cache.Std().Get(rdsKey, ret)
	if err == nil && ret.Value != "" {
		return ret, nil
	}

	err = dbtools.Core().Table(TableNameSettings).
		Where("`group` = ? AND `key` = ?", group, key).
		First(ret).Error
	if err != nil {
		logs.Errorf("[settings] get %s failed, %s", group+"/"+key, err)
		return nil, err
	}

	if ret.Value != "" {
		cache.Std().Set(rdsKey, ret, time.Minute*5)
	}

	return ret, nil
}

// GetValue 获取配置值，如果是加密配置，返回解密后的值
func GetValue(group, key string) (string, error) {
	si, err := Get(group, key)
	if err != nil {
		return "", err
	}
	return si.SecretValue(), nil
}

// GetYaml 获取yaml配置
func GetYaml(group, key string, value interface{}) error {
	text, err := GetValue(group, key)
	if err != nil {
		return err
	}
	return yaml.Unmarshal([]byte(text), value)
}

// GetText 获取文本配置
func GetText(group, key string) (string, error) {
	return GetValue(group, key)
}

// GetSecret 获取密码配置
func GetSecret(group, key string) (string, error) {
	si, err := Get(group, key)
	if err != nil {
		return "", err
	}
	return DecryptSecret(si.Value), nil
}

// GetSecretYaml 获取密码配置YAML
func GetSecretYaml(group, key string, value interface{}) error {
	date, err := GetSecret(group, key)
	if err != nil {
		return err
	}
	return yaml.Unmarshal([]byte(date), value)
}

// GetJSON 获取json配置
func GetJSON(group, key string, value interface{}) error {
	text, err := GetValue(group, key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(text), value)
}

// List 配置列表
func List(group string, keys ...string) ([]*SettingItem, error) {
	ret := []*SettingItem{}
	err := dbtools.Core().Table(TableNameSettings).
		Where("`group` = ? AND `key` IN (?)", group, keys).
		Find(&ret).Error
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// Updates 更新settings值
func Updates(sets ...*SettingItem) error {
	for _, set := range sets {
		sql := dbtools.Core().Table(TableNameSettings)
		if set.ID != 0 {
			sql = sql.Where("id = ?", set.ID)
		} else {
			sql = sql.Where("`group` = ? AND `key` = ?", set.Group, set.Key)
		}
		update := map[string]interface{}{
			"value":      set.Value,
			"value_type": set.ValueType,
			"describe":   set.Describe,
			"name":       set.Name,
		}
		err := sql.Updates(update).Error
		if err != nil {
			logs.Errorf("[settings] update %s failed, %s", set.Identify(), err)
			return err
		}
		rdsKey := redisCacheKey(set.Group, set.Key)
		cache.Std().Delete(rdsKey)
	}
	return nil
}

func redisCacheKey(group, key string) string {
	return fmt.Sprintf("core_setting::%s::%s", group, key)
}
