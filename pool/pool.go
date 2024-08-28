package pool

import (
	"container/list"
	"sync"
)

// Pool 资源池接口
type Pool interface {
	// Acquire 从资源池中获取一个资源
	Acquire() (interface{}, error)
	// Release 释放一个资源到资源池
	Release(interface{}) error
	// AcquireDecode 从资源池中获取一个资源
	AcquireDecode(v interface{}) error
	// ReleaseEncode 释放一个资源到资源池
	ReleaseEncode(v interface{}) error
	// Clear 清空资源池
	Clear()
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

// Clear 清空资源池
func (gp *GoPool) Clear() {
	gp.Lock()
	defer gp.Unlock()
	gp.p.Init()
}
