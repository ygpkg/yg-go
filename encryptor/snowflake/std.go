package snowflake

import (
	"math/rand"

	"github.com/mr-tron/base58"
)

var std = New(rand.Int63n(1 << nodeBits))

// SetNodeID 设置雪花ID的节点ID，范围为0-1023
func SetNodeID(nodeID int64) {
	if nodeID < 0 || nodeID > (1<<nodeBits-1) {
		panic("nodeID must be between 0 and 1023")
	}
	std.lock.Lock()
	defer std.lock.Unlock()
	std.nodeID = nodeID
}

// GenerateID 生成雪花ID
func GenerateID() uint64 {
	return std.Next()
}

// GenerateIDBase58 生成雪花ID并编码为Base58字符串
func GenerateIDBase58() string {
	id := GenerateID()
	buf := make([]byte, 8)

	for i := uint(0); i < 8; i++ {
		buf[7-i] = byte(id >> (i * 8))
	}

	return base58.Encode(buf)
}
