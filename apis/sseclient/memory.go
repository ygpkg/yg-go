package sseclient

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"time"
)

type memoryCache struct {
	dataMap        map[string]memoryDataItem
	signals        map[string]bool
	writeKeyPrefix string
	stopKeyPrefix  string
	mu             sync.RWMutex
}

type memoryDataItem struct {
	list    []string
	expired time.Time
}

func newMemoryCache(writeKeyPrefix, stopKeyPrefix string) *memoryCache {
	return &memoryCache{
		dataMap:        make(map[string]memoryDataItem),
		signals:        make(map[string]bool),
		writeKeyPrefix: writeKeyPrefix,
		stopKeyPrefix:  stopKeyPrefix,
	}
}

func (m *memoryCache) WriteMessage(ctx context.Context, key, msg string, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	list := m.dataMap[key].list
	list = append(list, msg)
	m.dataMap[key] = memoryDataItem{
		list:    list,
		expired: time.Now().Add(expiration),
	}
	return nil
}

func (m *memoryCache) ReadMessages(ctx context.Context, key string) (string, []string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if data, exists := m.dataMap[key]; exists {
		if data.expired.Before(time.Now()) {
			delete(m.dataMap, key)
			stoppedKey := strings.ReplaceAll(key, m.writeKeyPrefix, m.stopKeyPrefix)
			delete(m.signals, stoppedKey)
			return "", nil, nil
		}
		n := len(data.list)
		result := make([]string, n)
		copy(result, data.list)
		return strconv.Itoa(n - 1), result, nil
	}
	return "", nil, nil
}

func (m *memoryCache) ReadAfterID(ctx context.Context, key, id string) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	data, exists := m.dataMap[key]
	if !exists {
		return "", "", nil
	}

	if data.expired.Before(time.Now()) {
		delete(m.dataMap, key)
		stoppedKey := strings.ReplaceAll(key, m.writeKeyPrefix, m.stopKeyPrefix)
		delete(m.signals, stoppedKey)
		return "", "", nil
	}

	n := len(data.list)
	idx, _ := strconv.Atoi(id)
	if idx+1 >= n {
		return "", "", nil
	}
	return strconv.Itoa(idx + 1), data.list[idx+1], nil
}

func (m *memoryCache) Set(ctx context.Context, key string, expiration time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.signals[key] = true
	return nil
}

func (m *memoryCache) Exist(ctx context.Context, key string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.dataMap[key]
	return exists, nil
}

func (m *memoryCache) Delete(ctx context.Context, key string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.dataMap, key)
	return nil
}
