package portspool

import (
	"errors"
	"sync"
)

// PortPool 是端口池的结构体
type PortPool struct {
	mu        sync.Mutex
	available []int // 可用端口列表
	allocated []int // 已分配的端口列表
	maxPort   int   // 最大端口号
	minPort   int   // 最小端口号
}

// NewPortPool 创建一个新的端口池
func NewPortPool(minPort, maxPort int) *PortPool {
	if minPort > maxPort || minPort < 0 || maxPort > 65535 {
		panic(errors.New("无效的端口范围"))
		return nil
	}

	available := make([]int, maxPort-minPort+1)
	for i := minPort; i <= maxPort; i++ {
		available[i-minPort] = i
	}

	return &PortPool{
		available: available,
		allocated: make([]int, 0),
		maxPort:   maxPort,
		minPort:   minPort,
	}
}

// GetPort 从端口池中获取一个可用端口
func (p *PortPool) GetPort() (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.available) == 0 {
		return 0, errors.New("端口池已用尽")
	}

	port := p.available[0]
	p.available = p.available[1:]
	p.allocated = append(p.allocated, port)

	return port, nil
}

// ReturnPort 归还一个端口到端口池中
func (p *PortPool) ReturnPort(port int) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	index := -1
	for i, allocatedPort := range p.allocated {
		if port == allocatedPort {
			index = i
			break
		}
	}

	if index == -1 {
		return errors.New("端口未分配或已归还")
	}

	p.allocated = append(p.allocated[:index], p.allocated[index+1:]...)
	p.available = append(p.available, port)

	return nil
}
