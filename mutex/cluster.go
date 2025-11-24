package mutex

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/lifecycle"
	"github.com/ygpkg/yg-go/logs"
)

const clusterMutexExpiration = 30 * time.Second

// ClusterMutex 集群互斥锁
type ClusterMutex struct {
	cli      *redis.Client
	ctx      context.Context
	key      string
	interval time.Duration

	myName        string
	currentMaster string
}

// NewClusterMutex .
func NewClusterMutex(ctx context.Context, cli *redis.Client, name string) *ClusterMutex {
	cm := &ClusterMutex{
		ctx:      ctx,
		cli:      cli,
		key:      fmt.Sprintf("core:cluster_mutex:%s", name),
		myName:   lifecycle.OwnerID(),
		interval: 8*time.Second + time.Millisecond*time.Duration(rand.Int63n(6000)),
	}
	go cm.daemonRoutine()
	return cm
}

// IsMaster 是否是主节点
func (m *ClusterMutex) IsMaster() bool {
	isMaster := m.myName == m.currentMaster
	if !isMaster {
		logs.InfoContextf(m.ctx, "[ClusterMutex] master info not match, my name: %s, current master: %s", m.myName, m.currentMaster)
	}
	return isMaster
}

// daemonRoutine 后台守护协程
func (m *ClusterMutex) daemonRoutine() {
	m.tryPreempt()
	tc := time.NewTicker(m.interval)
	for {
		select {
		case <-tc.C:
			_, err := m.getCurrentMaster()
			if err != nil {
				// 争抢锁
				if err := m.tryPreempt(); err != nil {
					continue
				}

				continue
			}
			if m.IsMaster() {
				// 续约
				if err := m.relete(); err != nil {
					logs.ErrorContextf(m.ctx, "[%s] cluster mutex: %s relete error: %v", m.key, m.myName, err)
				}
			}

		case <-m.ctx.Done():
			tc.Stop()
			m.refund()
			logs.InfoContextf(m.ctx, "[%s] cluster mutex: %s stop", m.key, m.myName)
			return
		}

	}
}

// tryPreempt 争抢锁
func (m *ClusterMutex) tryPreempt() error {
	succ, err := m.cli.SetNX(m.ctx, m.key, m.myName, clusterMutexExpiration).Result()
	if err != nil {
		return err
	}
	if succ {
		m.currentMaster = m.myName
		logs.InfoContextf(m.ctx, "[%s] cluster mutex: %s become master", m.key, m.myName)
	}
	return nil
}

// relete 续约
func (m *ClusterMutex) relete() error {
	_, err := m.cli.Expire(m.ctx, m.key, clusterMutexExpiration).Result()
	if err != nil {
		return err
	}
	return nil
}

// refund 归还
func (m *ClusterMutex) refund() error {
	if !m.IsMaster() {
		return fmt.Errorf("not master")
	}

	_, err := m.cli.Del(context.Background(), m.key).Result()
	if err != nil {
		logs.ErrorContextf(m.ctx, "[%s] cluster mutex: %s refund error: %v", m.key, m.myName, err)
		return err
	}
	logs.InfoContextf(m.ctx, "[%s] cluster mutex: %s refund", m.key, m.myName)
	return nil
}

// getCurrentMaster 获取当前主节点
func (m *ClusterMutex) getCurrentMaster() (string, error) {
	// 获取当前主节点
	master, err := m.cli.Get(m.ctx, m.key).Result()
	if err != nil {
		return "", err
	}
	if master == "" {
		return "", fmt.Errorf("master not found")
	}
	m.currentMaster = master
	return master, nil
}
