package random

import (
	"math/rand"
	"time"
)

const (
	NUMBER   = "0123456789"
	ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	ALPHANUM = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	ALPHASYM = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz~!@#$%^&*_+?-="
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// RandString 生成随机字符串
func RandString(count int, set string) string {
	var (
		ret = make([]byte, count)
	)
	for i := 0; i < count; i++ {
		ret[i] = set[rand.Intn(len(set))]
	}
	return string(ret)
}

// Number 获取随机数字字符串
func Number(count int) string {
	var (
		ret = make([]byte, count)
	)
	for i := 0; i < count; i++ {
		ret[i] = byte(rand.Intn(10)) + byte('0')
	}
	return string(ret)
}

// Int return x start <= x <= end
func Int(start, end int) int {
	return rand.Intn(end-start+1) + start
}

// Alphabet 获取随机字母字符串
func Alphabet(count int) string {
	return RandString(count, ALPHABET)
}

// Alphanum 获取随机字母数字字符串
func Alphanum(count int) string {
	return RandString(count, ALPHANUM)
}

// Alphasym 获取随机字母数字特殊字符字符串
func Alphasym(count int) string {
	return RandString(count, ALPHASYM)
}

// String 获取随机字符串, 字符串由数字、字母组成,不包含特殊字符
func String(length int) string {
	return Alphanum(length)
}

// Uint 获取随机数
func Uint() uint {
	return uint(rand.Uint32())
}
