package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/ygpkg/yg-go/dbtools/redispool"
	"github.com/ygpkg/yg-go/task/model"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// setupInfra 初始化基础设施（数据库和 Redis）
func setupInfra() (*gorm.DB, *redis.Client, error) {
	db, err := setupDB()
	if err != nil {
		return nil, nil, fmt.Errorf("数据库连接失败: %w\n\n提示: 请确保 MySQL 服务正在运行", err)
	}

	redisClient, err := setupRedis()
	if err != nil {
		return nil, nil, fmt.Errorf("Redis 连接失败: %w\n\n提示: 请确保 Redis 服务正在运行", err)
	}

	redispool.InitCache(redisClient)

	return db, redisClient, nil
}

// setupDB 使用 gorm 原生方式创建数据库连接
func setupDB() (*gorm.DB, error) {
	// MySQL DSN 格式: user:pass@tcp(host:port)/dbname?charset=utf8mb4&parseTime=True&loc=Local
	dsn := "root:123456@tcp(localhost:3306)/demo?charset=utf8mb4&parseTime=True&loc=Local"

	fmt.Print("  连接 MySQL...")
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		CreateBatchSize: 200,
	})
	if err != nil {
		fmt.Println(" 失败")
		return nil, fmt.Errorf("failed to connect database: %w", err)
	}
	fmt.Println(" 成功")

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
	fmt.Print("  连接 Redis...")
	ctx := context.Background()
	if err := client.Ping(ctx).Err(); err != nil {
		fmt.Println(" 失败")
		return nil, fmt.Errorf("failed to connect redis: %w", err)
	}
	fmt.Println(" 成功")

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

func createDemoTask(ctx context.Context, taskMgr TaskCreator) error {
	payload := DemoPayload{
		Message: "Hello, Task System!",
		UserID:  1001,
	}
	payloadJSON, _ := json.Marshal(payload)

	task := &model.TaskEntity{
		TaskType:    "demo_task",
		TaskStatus:  model.TaskStatusPending,
		SubjectID:   1,
		SubjectType: "example",
		Payload:     string(payloadJSON),
		Timeout:     30 * time.Second,
		MaxRedo:     3,
	}

	return taskMgr.CreateTasks(ctx, []*model.TaskEntity{task})
}

type TaskCreator interface {
	CreateTasks(ctx context.Context, tasks []*model.TaskEntity) error
}
