package cache

import (
	"github.com/ygpkg/yg-go/cache/cachetype"
	"github.com/ygpkg/yg-go/cache/memory"
)

var std cachetype.Cache

// InitCache init cache
func InitCache(c cachetype.Cache) {
	std = c
}

// Std get std cache
func Std() cachetype.Cache {
	if std == nil {
		std = memory.NewCache()
	}
	return std
}

// WechatCache wechat cache
func WechatCache() *wechatCache {
	return &wechatCache{c: Std()}
}
