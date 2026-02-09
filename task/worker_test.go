package task

import (
	"context"
	"fmt"

	"github.com/go-redis/redis/v8"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// SetupDB 创建数据库连接
func SetupDB() (*gorm.DB, error) {
	dsn := "root:123456@tcp(localhost:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local"

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		CreateBatchSize: 200,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	return db, nil
}

// SetupRedis 创建 Redis 客户端
func SetupRedis() (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // 无密码
		DB:       0,  // 默认 DB
	})

	// 测试连接
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect redis: %w", err)
	}

	return client, nil
}
