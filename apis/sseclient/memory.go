package sseclient

import (
	"context"
	"sync"
	"time"
)

type memoryCache struct {
	data    map[string][]string
	signals map[string]bool
	mu      sync.RWMutex
}

func newMemoryCache() *memoryCache {
	return &memoryCache{
		data:    make(map[string][]string),
		signals: make(map[string]bool),
	}
}

func (m *memoryCache) WriteMessage(ctx context.Context, key, msg string, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = append(m.data[key], msg)
	return nil
}

func (m *memoryCache) ReadMessages(ctx context.Context, key string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if messages, exists := m.data[key]; exists {
		result := make([]string, len(messages))
		copy(result, messages)
		return result, nil
	}
	return []string{}, nil
}

func (m *memoryCache) SetStopSignal(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signals[key] = true
	return nil
}

func (m *memoryCache) GetStopSignal(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.signals[key], nil
}

func (m *memoryCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}
