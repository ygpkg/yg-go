package openai

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"time"

	oai "github.com/sashabaranov/go-openai"
	"github.com/ygpkg/yg-go/llm"
	"github.com/ygpkg/yg-go/llm/llmtype"
)

// DefaultHTTPTimeout is the default timeout for OpenAI HTTP client requests.
const DefaultHTTPTimeout = 120 * time.Second

// OpenAIClient OpenAI 驱动适配器，实现 llmtype.Client 接口
// 通过 sashabaranov/go-openai SDK 与兼容 OpenAI 协议的 API 通信
type OpenAIClient struct {
	client       *oai.Client
	defaultModel string
}

// Register registers the OpenAI provider into the global factory. Call openai.Register() explicitly to initialize.
func Register() {
	llm.RegisterProvider("openai", newOpenAIFactory)
}

// newOpenAIFactory OpenAI 驱动工厂函数
func newOpenAIFactory(apiKey string, opts ...llm.Option) (llmtype.Client, error) {
	cfg := &llm.Config{}
	for _, opt := range opts {
		opt.Apply(cfg)
	}

	clientConfig := oai.DefaultConfig(apiKey)
	if cfg.BaseURL != "" {
		clientConfig.BaseURL = cfg.BaseURL
	}

	if cfg.ProxyURL != "" {
		proxyURL, err := url.Parse(cfg.ProxyURL)
		if err != nil {
			return nil, errors.New("invalid proxy url: " + cfg.ProxyURL)
		}
		clientConfig.HTTPClient = &http.Client{
			Transport: &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			},
			Timeout: DefaultHTTPTimeout,
		}
	}

	client := oai.NewClientWithConfig(clientConfig)
	return &OpenAIClient{
		client:       client,
		defaultModel: cfg.ModelName,
	}, nil
}

// Chat 同步调用大模型，等待完整响应返回
func (c *OpenAIClient) Chat(ctx context.Context, req *llmtype.ChatRequest) (*llmtype.ChatResponse, error) {
	if req.Model == "" {
		req.Model = c.defaultModel
	}

	oaiReq := convertRequest(req)
	resp, err := c.client.CreateChatCompletion(ctx, oaiReq)
	if err != nil {
		return nil, err
	}

	return convertResponse(resp), nil
}

// ChatStream 流式调用大模型，通过回调函数逐块处理增量响应
func (c *OpenAIClient) ChatStream(ctx context.Context, req *llmtype.ChatRequest, handler llmtype.StreamHandler) error {
	if req.Model == "" {
		req.Model = c.defaultModel
	}

	oaiReq := convertRequest(req)
	stream, err := c.client.CreateChatCompletionStream(ctx, oaiReq)
	if err != nil {
		return err
	}

	return handleStream(stream, handler)
}

// isStreamEOF 判断流式响应是否已正常结束
func isStreamEOF(err error) bool {
	return errors.Is(err, io.EOF)
}
