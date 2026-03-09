package snowflake

import (
	"sync"
	"time"
)

const (
	nodeBits = 10
	seqBits  = 12

	maxSeq = (1 << seqBits) - 1

	nodeShift = seqBits
	timeShift = seqBits + nodeBits

	epoch = 1735689600000 // 2025-01-01
)

type Generator struct {
	nodeID   int64
	seq      int64
	lastTime int64
	lock     sync.Mutex
}

func New(nodeID int64) *Generator {
	return &Generator{
		nodeID: nodeID,
	}
}

func (g *Generator) Next() uint64 {
	g.lock.Lock()
	defer g.lock.Unlock()

	now := time.Now().UnixMilli()

	if now == g.lastTime {
		g.seq = (g.seq + 1) & maxSeq
		if g.seq == 0 {
			for now <= g.lastTime {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		g.seq = 0
	}

	g.lastTime = now

	id := ((now - epoch) << timeShift) |
		(g.nodeID << nodeShift) |
		g.seq

	return uint64(id)
}
