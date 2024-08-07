package memory

import (
	"fmt"
	"sync"
	"time"

	"github.com/ygpkg/yg-go/cache/cachetype"
)

var _ cachetype.Cache = (*Memory)(nil)

// Memory struct contains *memcache.Client
type Memory struct {
	sync.Mutex

	data map[string]*data
}

type data struct {
	Data    interface{}
	Expired time.Time
}

// NewCache create new memcache
func NewCache() *Memory {
	return &Memory{
		data: map[string]*data{},
	}
}

// Get return cached value
func (mem *Memory) Get(key string, val interface{}) error {
	if ret, ok := mem.data[key]; ok {
		if ret.Expired.Before(time.Now()) {
			mem.deleteKey(key)
			return fmt.Errorf("key(%s) expired at %s", key, ret.Expired)
		}
		val = ret.Data
	}
	return nil
}

// IsExist check value exists in memcache.
func (mem *Memory) IsExist(key string) bool {
	if ret, ok := mem.data[key]; ok {
		if ret.Expired.Before(time.Now()) {
			mem.deleteKey(key)
			return false
		}
		return true
	}
	return false
}

// Set cached value with key and expire time.
func (mem *Memory) Set(key string, val interface{}, timeout time.Duration) (err error) {
	mem.Lock()
	defer mem.Unlock()

	mem.data[key] = &data{
		Data:    val,
		Expired: time.Now().Add(timeout),
	}
	return nil
}

// Delete delete value in memcache.
func (mem *Memory) Delete(key string) error {
	return mem.deleteKey(key)
}

// deleteKey
func (mem *Memory) deleteKey(key string) error {
	mem.Lock()
	defer mem.Unlock()
	delete(mem.data, key)
	return nil
}
