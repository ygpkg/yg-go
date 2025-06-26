package sseclient

import (
	"context"
	"time"
)

type Cache interface {
	// WriteMessage 写入消息到指定流
	WriteMessage(ctx context.Context, key, msg string, expiration time.Duration) error
	// ReadMessages 读取指定流的消息
	ReadMessages(ctx context.Context, key string) ([]string, error)
	// SetStopSignal 设置停止信号
	SetStopSignal(ctx context.Context, key string) error
	// GetStopSignal 获取停止信号状态
	GetStopSignal(ctx context.Context, key string) (bool, error)
	// DeleteMessage 删除消息
	DeleteMessage(ctx context.Context, key string) error
}
