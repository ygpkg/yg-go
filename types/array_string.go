package types

import (
	"encoding/json"

	"github.com/ygpkg/yg-go/logs"
)

// StringArray 字符串数组，用于Mysql数据库和接口之间的转换
type StringArray string

// NewStringArray 新建字符串数组
func NewStringArray(v []string) StringArray {
	if len(v) == 0 {
		return StringArray("")
	}
	bs, _ := json.Marshal(v)
	return StringArray(bs)
}

// MarshalJSON .
func (i StringArray) MarshalJSON() ([]byte, error) {
	arr := i.Slice()
	return json.Marshal(arr)
}

// UnmarshalJSON .
func (i *StringArray) UnmarshalJSON(data []byte) error {
	it := StringArray(string(data))
	*i = it
	return nil
}

// MarshalYAML .
func (i StringArray) MarshalYAML() (interface{}, error) {
	arr := i.Slice()
	return arr, nil
}

// UnmarshalYAML .
func (i *StringArray) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var arr []string
	err := unmarshal(&arr)
	if err != nil {
		return err
	}
	*i = NewStringArray(arr)
	return nil
}

// Slice .
func (i StringArray) Slice() (arr []string) {
	arr = []string{}
	if i == "" {
		return
	}

	err := decodeJSON(string(i), &arr)
	if err != nil {
		logs.Errorf("array %s decode failed, %s", i, err)
		return
	}
	return
}

func decodeJSON(bs string, v interface{}) error {
	return json.Unmarshal([]byte(bs), v)
}

func (i StringArray) First() string {
	us := i.Slice()
	if len(us) > 0 {
		return us[0]
	}
	return ""
}

// Add 向 StringArray 中添加一个新元素
func (i *StringArray) Add(item string) {
	// 获取当前数组
	arr := i.Slice()

	// 追加新元素
	arr = append(arr, item)

	// 更新 StringArray
	*i = NewStringArray(arr)
}

// Remove 从 StringArray 中移除一个 uint 值
func (i *StringArray) Remove(item string) {
	arr := i.Slice()
	var newArray []string
	for _, v := range arr {
		if v != item {
			newArray = append(newArray, v)
		}
	}
	*i = NewStringArray(newArray)
}

// Contains 检查 StringArray 是否包含指定值
func (i StringArray) Contains(value string) bool {
	arr := i.Slice()
	for _, v := range arr {
		if v == value {
			return true
		}
	}
	return false
}

// RemoveDuplicates 移除重复值
func (i *StringArray) RemoveDuplicates() {
	arr := i.Slice()
	unique := make(map[string]bool)
	var result []string

	for _, v := range arr {
		if !unique[v] {
			unique[v] = true
			result = append(result, v)
		}
	}

	*i = NewStringArray(result)
}
