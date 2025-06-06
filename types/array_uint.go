package types

import (
	"encoding/json"
	"log"
)

// UintArray 用于 MySQL 数据库和接口之间的 uint 数组转换
type UintArray string

// NewUintArray 创建一个 UintArray
func NewUintArray(v []uint) UintArray {
	if len(v) == 0 {
		return UintArray("[]") // 空数组表示为 JSON 数组
	}
	bs, _ := json.Marshal(v)
	return UintArray(bs)
}

// MarshalJSON 实现 json.Marshaler 接口
func (i UintArray) MarshalJSON() ([]byte, error) {
	arr := i.Slice()
	return json.Marshal(arr)
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
func (i *UintArray) UnmarshalJSON(data []byte) error {
	it := UintArray(string(data))
	*i = it
	return nil
}

// Slice 将 UintArray 转换为 []uint
func (i UintArray) Slice() (arr []uint) {
	arr = []uint{}
	if i == "" || string(i) == "null" {
		return
	}

	err := decodeJSON(string(i), &arr)
	if err != nil {
		log.Printf("UintArray %s decode failed, %s", i, err)
		return
	}
	return
}

// Append 向 UintArray 中添加一个 uint 值
func (i *UintArray) Append(value uint) {
	arr := i.Slice()
	arr = append(arr, value)
	*i = NewUintArray(arr)
}

// Remove 从 UintArray 中移除一个 uint 值
func (i *UintArray) Remove(item uint) {
	arr := i.Slice()
	var newArray []uint
	for _, v := range arr {
		if v != item {
			newArray = append(newArray, v)
		}
	}
	*i = NewUintArray(newArray)
}

// Contains 检查 UintArray 是否包含指定值
func (i UintArray) Contains(value uint) bool {
	arr := i.Slice()
	for _, v := range arr {
		if v == value {
			return true
		}
	}
	return false
}

// RemoveDuplicates 移除重复值
func (i *UintArray) RemoveDuplicates() {
	arr := i.Slice()
	unique := make(map[uint]bool)
	var result []uint

	for _, v := range arr {
		if !unique[v] {
			unique[v] = true
			result = append(result, v)
		}
	}

	*i = NewUintArray(result)
}
