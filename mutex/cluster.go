package mutex

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/logs"
)

const (
	// 锁过期时间（必须 > 续约周期 * 2）
	clusterMutexExpiration = 30 * time.Second
	// 最小续约间隔
	minRenewInterval = 8 * time.Second
)

// ClusterMutex 集群互斥锁
type ClusterMutex struct {
	ctx    context.Context
	cli    *redis.Client
	key    string
	myName string

	currentMaster string

	interval time.Duration
	ready    chan struct{}
}

// NewClusterMutex 创建 ClusterMutex
func NewClusterMutex(
	ctx context.Context,
	cli *redis.Client,
	lockName string,
	nodeID string,
) *ClusterMutex {

	m := &ClusterMutex{
		ctx:      ctx,
		cli:      cli,
		key:      fmt.Sprintf("cluster:mutex:%s", lockName),
		myName:   nodeID,
		ready:    make(chan struct{}),
		interval: minRenewInterval + time.Millisecond*time.Duration(rand.Int63n(5000)),
	}

	go m.daemonRoutine()
	return m
}

// IsMaster 当前实例是否为主节点
func (m *ClusterMutex) IsMaster() bool {
	return m.myName != "" && m.myName == m.currentMaster
}

// WaitReady 等待首次选主完成（用于启动定时任务前）
func (m *ClusterMutex) WaitReady(ctx context.Context) error {
	select {
	case <-m.ready:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// daemonRoutine 后台守护协程
func (m *ClusterMutex) daemonRoutine() {
	// 启动即尝试选主
	m.tryPreempt(true)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.tick()
		case <-m.ctx.Done():
			m.release()
			logs.InfoContextf(m.ctx, "[cluster_mutex] %s stopped", m.myName)
			return
		}
	}
}

// tick 周期任务
func (m *ClusterMutex) tick() {
	master, err := m.getCurrentMaster()
	if err != nil {
		// 无主节点，尝试抢锁
		_ = m.tryPreempt(false)
		return
	}

	if master == m.myName {
		// 我是主节点，续约
		if err := m.renew(); err != nil {
			logs.ErrorContextf(m.ctx, "[cluster_mutex] renew failed: %v", err)
		}
	}
}

// tryPreempt 抢占主节点
func (m *ClusterMutex) tryPreempt(first bool) error {
	ok, err := m.cli.SetNX(
		m.ctx,
		m.key,
		m.myName,
		clusterMutexExpiration,
	).Result()

	if err != nil {
		return err
	}

	if ok {
		m.currentMaster = m.myName
		logs.InfoContextf(m.ctx, "[cluster_mutex] %s become master", m.myName)
		m.markReady()
	} else if first {
		// 首次未抢到，也要标记 ready（已有主）
		_, _ = m.getCurrentMaster()
		m.markReady()
	}

	return nil
}

// renew 续约主节点锁
func (m *ClusterMutex) renew() error {
	return m.cli.Expire(
		m.ctx,
		m.key,
		clusterMutexExpiration,
	).Err()
}

// release 主动释放锁（仅主节点）
func (m *ClusterMutex) release() {
	if !m.IsMaster() {
		return
	}
	if err := m.cli.Del(context.Background(), m.key).Err(); err != nil {
		logs.ErrorContextf(m.ctx, "[cluster_mutex] release failed: %v", err)
	}
}

// getCurrentMaster 获取当前主节点
func (m *ClusterMutex) getCurrentMaster() (string, error) {
	val, err := m.cli.Get(m.ctx, m.key).Result()
	if err != nil {
		return "", err
	}
	m.currentMaster = val
	return val, nil
}

// markReady 标记首次选主完成
func (m *ClusterMutex) markReady() {
	select {
	case <-m.ready:
	default:
		close(m.ready)
	}
}
