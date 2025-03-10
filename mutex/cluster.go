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

type ClusterMutex struct {
	cli      *redis.Client
	ctx      context.Context
	key      string
	interval time.Duration

	myName        string
	currentMaster string
}

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
	return m.myName == m.currentMaster
}

// daemonRoutine 后台守护协程
func (m *ClusterMutex) daemonRoutine() {
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
				logs.InfoContextf(m.ctx, "cluster mutex: %s become master", m.myName)
				continue
			}
			if m.IsMaster() {
				// 续约
				if err := m.relete(); err != nil {
					logs.ErrorContextf(m.ctx, "cluster mutex: %s relete error: %v", m.myName, err)
					continue
				}
			}

		case <-m.ctx.Done():
			tc.Stop()
			m.refund()
			logs.InfoContextf(m.ctx, "cluster mutex: %s stop", m.myName)
			return
		}

	}
}

// tryPreempt 争抢锁
func (m *ClusterMutex) tryPreempt() error {
	return m.cli.SetNX(m.ctx, m.key, m.myName, clusterMutexExpiration).Err()
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
	_, err := m.cli.Del(m.ctx, m.key).Result()
	if err != nil {
		return err
	}
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
