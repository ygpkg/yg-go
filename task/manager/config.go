package manager

import (
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

// QueueConfig 队列配置
type QueueConfig struct {
	// KeyPrefix Redis 键前缀
	KeyPrefix string
	// BlockTime 阻塞读取时间
	BlockTime time.Duration
	// MaxRetries 最大重试次数（预留字段）
	MaxRetries int
	// RedisClient Redis 客户端
	RedisClient *redis.Client
	// DB 数据库连接（预留字段，供未来扩展使用）
	DB *gorm.DB
}

// DefaultQueueConfig 返回默认队列配置
func DefaultQueueConfig() *QueueConfig {
	return &QueueConfig{
		KeyPrefix:  "task:",
		BlockTime:  5 * time.Second,
		MaxRetries: 3,
	}
}

// Validate 验证配置
func (c *QueueConfig) Validate() error {
	if c.KeyPrefix == "" {
		return fmt.Errorf("queue config: key prefix cannot be empty")
	}
	if c.RedisClient == nil {
		return fmt.Errorf("queue config: redis client is required")
	}
	if c.BlockTime <= 0 {
		c.BlockTime = 5 * time.Second // 使用默认值
	}
	if c.MaxRetries < 0 {
		c.MaxRetries = 3 // 使用默认值
	}
	return nil
}

// ManagerConfig 任务管理器配置
type ManagerConfig struct {
	// KeyPrefix Redis 键前缀
	KeyPrefix string
	// QueueBlockTime Redis Stream 阻塞时间
	QueueBlockTime time.Duration
	// QueueSyncInterval 队列同步间隔，默认 1 分钟
	QueueSyncInterval time.Duration
}

// DefaultManagerConfig 返回默认管理器配置
func DefaultManagerConfig() *ManagerConfig {
	return &ManagerConfig{
		KeyPrefix:         "task:",
		QueueBlockTime:    5 * time.Second,
		QueueSyncInterval: time.Minute,
	}
}

// Validate 验证配置
func (c *ManagerConfig) Validate() error {
	if c.KeyPrefix == "" {
		return fmt.Errorf("manager config: key prefix cannot be empty")
	}
	if c.QueueBlockTime <= 0 {
		c.QueueBlockTime = 5 * time.Second
	}
	if c.QueueSyncInterval <= 0 {
		c.QueueSyncInterval = time.Minute
	}
	return nil
}
