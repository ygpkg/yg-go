package settings

import (
	"encoding/json"
	"fmt"

	"github.com/ygpkg/yg-go/dbutil"
	"github.com/ygpkg/yg-go/logs"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	TableNameSettings = "core_settings"
)

type ValueType string

const (
	ValueSecret ValueType = "secret"
	ValueText   ValueType = "text"
	ValueInt64  ValueType = "int64"
	ValueBool   ValueType = "bool"
	ValueJSON   ValueType = "json"
	ValueYaml   ValueType = "yaml"
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

// Identify 唯一建
func (item SettingItem) Identify() string {
	return fmt.Sprintf("%s/%s", item.Group, item.Key)
}

// PasswordValue 密码原文
func (item *SettingItem) PasswordValue() string {
	if item.ValueType == ValueSecret {
		return DecryptPassword(item.Value)
	}
	return item.Value
}

// InitDB .
func InitDB() error {
	return dbutil.InitModel(dbutil.Core(), &SettingItem{})
}

// GetByID .
func GetByID(id uint) (*SettingItem, error) {
	ret := &SettingItem{}
	err := dbutil.Core().Table(TableNameSettings).
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
	return updateSettings(si)
}

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
	return updateSettings(si)
}

// updateSettings or update the trade calendar of a stock.
func updateSettings(v *SettingItem) error {
	return dbutil.Core().Table(TableNameSettings).
		Clauses(clause.OnConflict{
			DoUpdates: clause.AssignmentColumns([]string{"name", "describe", "value", "value_type", "default"}),
		}).Create(v).Error
}

// Get .
func Get(group, key string) (*SettingItem, error) {
	ret := &SettingItem{}
	err := dbutil.Core().Table(TableNameSettings).
		Where("`group` = ? AND `key` = ?", group, key).
		First(ret).Error
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// GetYaml 获取yaml配置
func GetYaml(group, key string, value interface{}) error {
	si, err := Get(group, key)
	if err != nil {
		return err
	}
	return yaml.Unmarshal([]byte(si.Value), value)
}

// GetText 获取文本配置
func GetText(group, key string) (string, error) {
	si, err := Get(group, key)
	if err != nil {
		return "", err
	}
	return si.Value, nil
}

// GetPassword 获取密码配置
func GetPassword(group, key string) (string, error) {
	si, err := Get(group, key)
	if err != nil {
		return "", err
	}
	return DecryptPassword(si.Value), nil
}

// GetJSON 获取json配置
func GetJSON(group, key string, value interface{}) error {
	si, err := Get(group, key)
	if err != nil {
		return err
	}
	return json.Unmarshal([]byte(si.Value), value)
}

// List 配置列表
func List(group string, keys ...string) ([]*SettingItem, error) {
	ret := []*SettingItem{}
	err := dbutil.Core().Table(TableNameSettings).
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
		sql := dbutil.Core().Table(TableNameSettings)
		if set.ID != 0 {
			sql = sql.Where("id = ?", set.ID)
		} else {
			sql = sql.Where("`group` = ? AND `key` = ?", set.Group, set.Key)
		}
		if set.ValueType == ValueSecret {
			set.Value = EncryptPassword(set.Value)
		}
		err := sql.Update("value", set.Value).Error
		if err != nil {
			logs.Errorf("[settings] update %s failed, %s", set.Identify(), err)
			return err
		}
	}
	return nil
}
