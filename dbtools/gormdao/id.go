package gormdao

import "reflect"

// IDType 主键 ID 类型约束：支持无符号/有符号整型与字符串。
// 仅 Dao 以该约束作为类型参数，从而一套代码同时支持 uint 与 string 主键。
type IDType interface {
	~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~int | ~int8 | ~int16 | ~int32 | ~int64 | ~string
}

// isZeroAny 判断任意值是否为零值：nil 视为零值；否则用反射判断
// （整型零值为 0、字符串零值为 ""，含命名类型）。
func isZeroAny(v any) bool {
	if v == nil {
		return true
	}
	return reflect.ValueOf(v).IsZero()
}
