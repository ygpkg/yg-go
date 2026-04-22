package llm

import (
	"fmt"
	"sync"

	"github.com/ygpkg/yg-go/llm/llmtype"
)

// ProviderFactory 驱动工厂函数签名，由各 provider 驻动在 init() 中注册
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

// NewClient 根据驱动名称创建 LLM 客户端实例
// provider: 驱动名称，如 "openai"、"deepseek"、"qwen"
// apiKey: API 密钥
// opts: 可选配置项（WithBaseURL / WithProxy / WithDefaultModel 等）
func NewClient(provider string, apiKey string, opts ...Option) (llmtype.Client, error) {
	mu.RLock()
	factory, ok := providers[provider]
	mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("unsupported llm provider: %s", provider)
	}
	return factory(apiKey, opts...)
}
