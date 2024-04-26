package cache

import (
	"encoding/json"
	"time"

	"github.com/ygpkg/yg-go/utils/logs"
)

// Cache interface
type Cache interface {
	Get(key string, val interface{}) error
	Set(key string, val interface{}, timeout time.Duration) error
	IsExist(key string) bool
	Delete(key string) error
}

func Marshal(val interface{}) string {
	bs, err := json.Marshal(val)
	if err != nil {
		logs.Errorf("[cache] marshal %T failed, %s", val, err)
		return ""
	}
	return string(bs)
}

func Unmarshal(data []byte, val interface{}) error {
	return json.Unmarshal(data, val)
}
