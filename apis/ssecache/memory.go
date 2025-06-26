package ssecache

import (
	"context"
	"sync"
	"time"
)

type memoryStorage struct {
	data    map[string][]string
	signals map[string]bool
	mu      sync.RWMutex
}

func newMemoryStorage() *memoryStorage {
	return &memoryStorage{
		data:    make(map[string][]string),
		signals: make(map[string]bool),
	}
}

func (m *memoryStorage) WriteMessage(ctx context.Context, key string, msg string, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data[key] = append(m.data[key], msg)
	return nil
}

func (m *memoryStorage) ReadMessages(ctx context.Context, key string, lastID string) ([]string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if messages, exists := m.data[key]; exists {
		result := make([]string, len(messages))
		copy(result, messages)
		return result, nil
	}
	return []string{}, nil
}

func (m *memoryStorage) SetStopSignal(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signals[key] = true
	return nil
}

func (m *memoryStorage) GetStopSignal(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.signals[key], nil
}

func (m *memoryStorage) DeleteMessage(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.data, key)
	return nil
}
