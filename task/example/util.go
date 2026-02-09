package main

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// setupInfra 初始化基础设施（数据库和 Redis）
func setupInfra() (*gorm.DB, *redis.Client, error) {
	// 初始化数据库
	db, err := setupDB()
	if err != nil {
		return nil, nil, fmt.Errorf("数据库连接失败: %w\n\n提示: 请确保 MySQL 服务正在运行", err)
	}

	// 初始化 Redis
	redisClient, err := setupRedis()
	if err != nil {
		return nil, nil, fmt.Errorf("Redis 连接失败: %w\n\n提示: 请确保 Redis 服务正在运行", err)
	}

	return db, redisClient, nil
}

// setupDB 使用 gorm 原生方式创建数据库连接
func setupDB() (*gorm.DB, error) {
	// MySQL DSN 格式: user:pass@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	dsn := "root:123456@tcp(localhost:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local"

	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		CreateBatchSize: 200,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}

	return db, nil
}

// setupRedis 使用 go-redis 原生方式创建 Redis 客户端
func setupRedis() (*redis.Client, error) {
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

// printSection 打印分隔线和标题
func printSection(title string) {
	fmt.Println()
	fmt.Println("========================================")
	fmt.Println(title)
	fmt.Println("========================================")
	fmt.Println()
}
