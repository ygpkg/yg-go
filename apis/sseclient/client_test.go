package sseclient

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
)

func TestWriteMessage(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	client := New(WithRedisClient(rdb), WithExpiration(60*time.Second))
	for i := 10; i < 20; i++ {
		stopped, err := client.WriteMessage(context.Background(), nil, "202506262238", fmt.Sprintf("value%d", i))
		assert.Nil(t, err)
		assert.False(t, stopped)
	}
}

func TestReadMessages(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	client := New(WithRedisClient(rdb), WithExpiration(60*time.Second))
	latestID, messages, err := client.ReadMessages(context.Background(), "202506262238")
	assert.Nil(t, err)
	t.Log("latestID:", latestID) // 1750948824775-0
	t.Log("messages:", messages)
}

func TestBlockRead(t *testing.T) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "",
		DB:       0,
	})
	client := New(WithRedisClient(rdb), WithExpiration(60*time.Second),WithBlockMaxRetry(10),WithBlockTimeout(time.Hour))
	ended, affectedRows, err := client.BlockRead(context.Background(), nil, "202506262238", "1750952999597-0")
	assert.Nil(t, err)
	assert.True(t, ended)
	t.Log("affectedRows:", affectedRows)
}

func TestWithMemory(t *testing.T) {
	client := New()
	for i := 0; i < 20; i++ {
		stopped, err := client.WriteMessage(context.Background(), nil, "202506262238-m", fmt.Sprintf("value%d", i))
		assert.Nil(t, err)
		assert.False(t, stopped)
	}

	latestID, messages, err := client.ReadMessages(context.Background(), "202506262238-m")
	assert.Nil(t, err)
	t.Log("latestID:", latestID)
	t.Log("messages:", messages)

	ended, affectedRows, err := client.BlockRead(context.Background(), nil, "202506262238-m", "9")
	assert.Nil(t, err)
	assert.True(t, ended)
	t.Log("affectedRows:", affectedRows)
}
