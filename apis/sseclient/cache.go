package sseclient

import (
	"context"
	"time"
)

type Cache interface {
	// WriteMessage 写入消息到指定流
	WriteMessage(ctx context.Context, key, msg string, expiration time.Duration) error
	// ReadMessages 读取指定流的消息
	ReadMessages(ctx context.Context, key string) (string, []string, error)
	ReadAfterID(ctx context.Context, key, id string) (string, string, error)
	// Set 设置数据
	Set(ctx context.Context, key string, expiration time.Duration) error
	// Exist 检查数据是否存在
	Exist(ctx context.Context, key string) (bool, error)
	// Delete 删除数据
	Delete(ctx context.Context, key string) error
}
