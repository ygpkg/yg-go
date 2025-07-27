package estool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

// ScrollConfig 滚动查询配置
type ScrollConfig struct {
	ScrollSize      int                          // 每批返回的文档数量，默认1000
	ScrollTime      time.Duration                // 滚动上下文保持时间，默认5分钟
	RespectMaxTotal bool                         // 是否限制总返回数，默认启用
	SearchOptions   []func(*esapi.SearchRequest) // 额外ES搜索请求配置函数列表
}

// ScrollOption 配置选项函数
type ScrollOption func(*ScrollConfig)

// WithScrollSize 设置滚动批次大小
func WithScrollSize(size int) ScrollOption {
	return func(c *ScrollConfig) {
		c.ScrollSize = size
	}
}

// WithScrollTime 设置滚动上下文保持时间
func WithScrollTime(scrollTime time.Duration) ScrollOption {
	return func(c *ScrollConfig) {
		c.ScrollTime = scrollTime
	}
}

// WithRespectMaxTotal 设置是否限制最大总返回数
func WithRespectMaxTotal(enable bool) ScrollOption {
	return func(c *ScrollConfig) {
		c.RespectMaxTotal = enable
	}
}

// WithSearchOptions 设置额外的 esapi.SearchRequest 配置函数
func WithSearchOptions(opts ...func(*esapi.SearchRequest)) ScrollOption {
	return func(c *ScrollConfig) {
		c.SearchOptions = append(c.SearchOptions, opts...)
	}
}

// defaultScrollConfig 返回默认配置
func defaultScrollConfig() ScrollConfig {
	return ScrollConfig{
		ScrollSize:      1000,
		ScrollTime:      5 * time.Minute,
		RespectMaxTotal: true,
		SearchOptions:   nil,
	}
}

// scrollResult 内部滚动查询结果
type scrollResult struct {
	ScrollID string     `json:"_scroll_id"`
	Hits     scrollHits `json:"hits"`
	TimedOut bool       `json:"timed_out"`
	Took     int        `json:"took"`
}

// scrollHits 内部命中结果
type scrollHits struct {
	Total    scrollTotal       `json:"total"`
	MaxScore *float64          `json:"max_score"`
	Hits     []json.RawMessage `json:"hits"`
}

// scrollTotal 内部总数信息
type scrollTotal struct {
	Value    int64  `json:"value"`
	Relation string `json:"relation"`
}

type ScrollSearch struct {
	client *elasticsearch.Client
}

// NewScrollSearch 创建滚动查询客户端
func NewScrollSearch(client *elasticsearch.Client) *ScrollSearch {
	return &ScrollSearch{
		client: client,
	}
}

// ScrollAll 执行完整滚动查询
// index: 索引名
// queryBody: 查询DSL字符串
// totalSize: 期望获取的总文档数，必填
// dest: 结果切片指针
// opts: 滚动配置选项
func (c *ScrollSearch) ScrollAll(
	ctx context.Context,
	index string,
	queryBody string,
	totalSize int,
	dest interface{},
	opts ...ScrollOption,
) error {
	if index == "" {
		return fmt.Errorf("索引名称不能为空")
	}
	if totalSize <= 0 {
		return fmt.Errorf("totalSize必须大于0")
	}

	config := defaultScrollConfig()
	for _, opt := range opts {
		opt(&config)
	}

	// 如果 ScrollSize 大于 totalSize，以 totalSize 为准
	if config.ScrollSize > totalSize {
		config.ScrollSize = totalSize
	}

	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr || destValue.Elem().Kind() != reflect.Slice {
		return fmt.Errorf("dest必须是指向切片的指针")
	}
	sliceValue := destValue.Elem()
	elemType := sliceValue.Type().Elem()

	// 构造 SearchRequest 并应用配置函数
	req := esapi.SearchRequest{
		Index:  []string{index},
		Scroll: config.ScrollTime,
		Size:   &config.ScrollSize,
		Body:   strings.NewReader(queryBody),
	}
	for _, fn := range config.SearchOptions {
		fn(&req)
	}

	// 执行初始搜索
	result, err := c.initialSearch(ctx, &req)
	if err != nil {
		return fmt.Errorf("初始搜索失败: %w", err)
	}

	scrollID := result.ScrollID
	defer func() {
		if scrollID != "" {
			c.clearScroll(ctx, scrollID)
		}
	}()

	totalProcessed := 0

	// 处理初始结果
	if len(result.Hits.Hits) > 0 {
		batchCount := len(result.Hits.Hits)
		if config.RespectMaxTotal && totalProcessed+batchCount > totalSize {
			batchCount = totalSize - totalProcessed
		}
		if batchCount > 0 {
			if err := c.appendHitsToSlice(result.Hits.Hits[:batchCount], sliceValue, elemType); err != nil {
				return fmt.Errorf("转换初始结果失败: %w", err)
			}
			totalProcessed += batchCount
		}
		if config.RespectMaxTotal && totalProcessed >= totalSize {
			return nil
		}
	}

	// 继续滚动查询
	for len(result.Hits.Hits) > 0 {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		result, err = c.continueScroll(ctx, scrollID, config)
		if err != nil {
			return fmt.Errorf("继续滚动查询失败: %w", err)
		}
		if result.ScrollID != "" {
			scrollID = result.ScrollID
		}

		batchCount := len(result.Hits.Hits)
		if config.RespectMaxTotal && totalProcessed+batchCount > totalSize {
			batchCount = totalSize - totalProcessed
		}

		if batchCount > 0 {
			if err := c.appendHitsToSlice(result.Hits.Hits[:batchCount], sliceValue, elemType); err != nil {
				return fmt.Errorf("转换滚动结果失败: %w", err)
			}
			totalProcessed += batchCount
		}

		if config.RespectMaxTotal && totalProcessed >= totalSize {
			return nil
		}
	}

	return nil
}

func (c *ScrollSearch) appendHitsToSlice(hits []json.RawMessage, sliceValue reflect.Value, elemType reflect.Type) error {
	for _, hit := range hits {
		var hitDoc struct {
			Source json.RawMessage `json:"_source"`
			ID     string          `json:"_id"`
			Index  string          `json:"_index"`
			Type   string          `json:"_type"`
		}
		if err := json.Unmarshal(hit, &hitDoc); err != nil {
			return fmt.Errorf("解析hit失败: %w", err)
		}
		newItem := reflect.New(elemType)
		if err := json.Unmarshal(hitDoc.Source, newItem.Interface()); err != nil {
			return fmt.Errorf("转换为目标类型失败: %w", err)
		}
		sliceValue.Set(reflect.Append(sliceValue, newItem.Elem()))
	}
	return nil
}

func (c *ScrollSearch) initialSearch(ctx context.Context, req *esapi.SearchRequest) (*scrollResult, error) {
	res, err := req.Do(ctx, c.client)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("搜索请求失败: %s", res.String())
	}

	var result scrollResult
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析搜索结果失败: %w", err)
	}
	return &result, nil
}

func (c *ScrollSearch) continueScroll(ctx context.Context, scrollID string, config ScrollConfig) (*scrollResult, error) {
	res, err := c.client.Scroll(
		c.client.Scroll.WithContext(ctx),
		c.client.Scroll.WithScrollID(scrollID),
		c.client.Scroll.WithScroll(config.ScrollTime),
	)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.IsError() {
		return nil, fmt.Errorf("滚动请求失败: %s", res.String())
	}

	var result scrollResult
	if err := json.NewDecoder(res.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析滚动结果失败: %w", err)
	}
	return &result, nil
}

func (c *ScrollSearch) clearScroll(ctx context.Context, scrollID string) {
	res, err := c.client.ClearScroll(
		c.client.ClearScroll.WithContext(ctx),
		c.client.ClearScroll.WithScrollID(scrollID),
	)
	if err != nil {
		return
	}
	defer res.Body.Close()
}
