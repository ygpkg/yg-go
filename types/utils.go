package types

import (
	"fmt"
	"reflect"
	"strconv"
)

// ContainsString 判断字符串是否在字符串数组中
func ContainsString(arr []string, str string) bool {
	for _, v := range arr {
		if v == str {
			return true
		}
	}
	return false
}

// InUintSlice 判断整数是否在整数数组中
func InUintSlice(num uint, arr []uint) bool {
	for _, v := range arr {
		if v == num {
			return true
		}
	}
	return false
}

// InStringSlice 判断字符串是否在字符串数组中
func InStringSlice(str string, arr []string) bool {
	for _, v := range arr {
		if v == str {
			return true
		}
	}
	return false
}

// MustString 将任意类型转换为字符串
func MustString(s interface{}) string {
	if s == nil {
		return ""
	}
	switch t := reflect.ValueOf(s); t.Kind() {
	case reflect.String:
		return s.(string)
	case reflect.Ptr:
		if t.IsNil() {
			return ""
		}

		switch t.Elem().Kind() {
		case reflect.String:
			return *(s.(*string))
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			return strconv.Itoa(*(s.(*int)))
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			return fmt.Sprint(*(s.(*uint)))
		case reflect.Float32, reflect.Float64:
			return fmt.Sprint(*(s.(*float64)))
		default:
			return ""
		}
	default:
		return fmt.Sprint(s)
	}
}
