package pool

import (
	"container/list"
	"sync"

	"github.com/ygpkg/yg-go/config"
)

// Pool 资源池接口
type Pool interface {

	// AcquireDecode 从资源池中获取一个资源
	AcquireDecode(v interface{}) error
	// ReleaseEncode 释放一个资源到资源池
	ReleaseEncode(v interface{}) error
	// AcquireString 从资源池中获取一个资源
	AcquireString() (string, error)
	// ReleaseString 释放一个资源到资源池
	ReleaseString(string) error
	// Clear 清空资源池
	Clear()
	// RefreshConfigs 刷新配置
	RefreshConfig(config.ServicePoolConfig) error
}

var _ Pool = (*GoPool)(nil)

// GoPool golang资源池
type GoPool struct {
	sync.Mutex
	p *list.List
}

// NewGoPool 创建一个golang资源池
func NewGoPool() *GoPool {
	return &GoPool{
		p: list.New(),
	}
}

// Acquire 从资源池中获取一个资源
func (gp *GoPool) Acquire() (interface{}, error) {
	gp.Lock()
	defer gp.Unlock()
	if gp.p.Len() == 0 {
		return nil, nil
	}
	v := gp.p.Front().Value
	gp.p.Remove(gp.p.Front())
	return v, nil
}

// AcquireDecode 从资源池中获取一个资源, 并解析到v
func (gp *GoPool) AcquireDecode(v interface{}) error {
	val, err := gp.Acquire()
	if err != nil {
		return err
	}
	v = val
	return nil
}

// Release 释放一个资源到资源池
func (gp *GoPool) Release(v interface{}) error {
	gp.Lock()
	defer gp.Unlock()
	gp.p.PushBack(v)
	return nil
}

// ReleaseEncode 释放一个资源到资源池
func (gp *GoPool) ReleaseEncode(v interface{}) error {
	return gp.Release(v)
}

// AcquireString 从资源池中获取一个资源
func (gp *GoPool) AcquireString() (string, error) {
	v, err := gp.Acquire()
	if err != nil {
		return "", err
	}
	return v.(string), nil
}

// ReleaseString 释放一个资源到资源池
func (gp *GoPool) ReleaseString(v string) error {
	return gp.Release(v)
}

// Clear 清空资源池
func (gp *GoPool) Clear() {
	gp.Lock()
	defer gp.Unlock()
	gp.p.Init()
}

// RefreshConfigs 刷新配置
func (gp *GoPool) RefreshConfig(config.ServicePoolConfig) error {
	return nil
}
