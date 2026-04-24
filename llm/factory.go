package llm

import (
	"fmt"
	"sync"

	"github.com/ygpkg/yg-go/llm/llmtype"
)

// ProviderFactory is the factory function signature for creating an LLM client instance.
type ProviderFactory func(apiKey string, opts ...Option) (llmtype.Client, error)

var (
	providers = make(map[string]ProviderFactory)
	mu        sync.RWMutex
)

// RegisterProvider 注册驱动工厂函数，由各 provider 驱动在 init() 中调用
func RegisterProvider(name string, factory ProviderFactory) {
	mu.Lock()
	defer mu.Unlock()
	providers[name] = factory
}

// NewClient creates an LLM client instance by provider name.
func NewClient(provider string, apiKey string, opts ...Option) (llmtype.Client, error) {
	mu.RLock()
	factory, ok := providers[provider]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported llm provider: %s", provider)
	}
	return factory(apiKey, opts...)
}
