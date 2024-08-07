package cache

import (
	"time"

	"github.com/ygpkg/yg-go/cache/cachetype"
)

type wechatCache struct {
	c cachetype.Cache
}

// // Cache interface
// type Cache interface {
// 	Get(key string) interface{}
// 	Set(key string, val interface{}, timeout time.Duration) error
// 	IsExist(key string) bool
// 	Delete(key string) error
// }

// Get value from cache
func (wc *wechatCache) Get(key string) interface{} {
	var val interface{}
	wc.c.Get(key, val)
	return val
}

// Set value to cache
func (wc *wechatCache) Set(key string, val interface{}, timeout time.Duration) error {
	return wc.c.Set(key, val, timeout)
}

// IsExist check value exists in cache.
func (wc *wechatCache) IsExist(key string) bool {
	return wc.c.IsExist(key)
}

// Delete value in cache.
func (wc *wechatCache) Delete(key string) error {
	return wc.c.Delete(key)
}
