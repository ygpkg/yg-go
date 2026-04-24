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

// DefaultHTTPTimeout is the default timeout for outbound HTTP requests.
const DefaultHTTPTimeout = 120 * time.Second

// Client adapts the sashabaranov/go-openai SDK to implement the llmtype.Client interface.
// It communicates with APIs compatible with the OpenAI protocol.
type Client struct {
	client       *oai.Client
	defaultModel string
}

// Register registers the OpenAI provider into the global factory. Call openai.Register() explicitly to initialize.
func Register() {
	llm.RegisterProvider("openai", newFactory)
}

// newFactory creates a Client instance using the given API key and options.
func newFactory(apiKey string, opts ...llm.Option) (llmtype.Client, error) {
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
	return &Client{
		client:       client,
		defaultModel: cfg.ModelName,
	}, nil
}

// Chat 同步调用大模型，等待完整响应返回
func (c *Client) Chat(ctx context.Context, req *llmtype.ChatRequest) (*llmtype.ChatResponse, error) {
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
func (c *Client) ChatStream(ctx context.Context, req *llmtype.ChatRequest, handler llmtype.StreamHandler) error {
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
