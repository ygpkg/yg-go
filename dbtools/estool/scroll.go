package estool

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/elastic/go-elasticsearch/v8"
)

// ScrollConfig 滚动查询配置
type ScrollConfig struct {
	Size       int           // 每批返回的文档数量，默认1000
	ScrollTime time.Duration // 滚动上下文保持时间，默认5分钟
	Timeout    time.Duration // 请求超时时间，默认30秒
}

// ScrollOption 配置选项函数
type ScrollOption func(*ScrollConfig)

// WithSize 设置每批返回的文档数量
func WithSize(size int) ScrollOption {
	return func(c *ScrollConfig) {
		c.Size = size
	}
}

// WithScrollTime 设置滚动上下文保持时间
func WithScrollTime(scrollTime time.Duration) ScrollOption {
	return func(c *ScrollConfig) {
		c.ScrollTime = scrollTime
	}
}

// WithTimeout 设置请求超时时间
func WithTimeout(timeout time.Duration) ScrollOption {
	return func(c *ScrollConfig) {
		c.Timeout = timeout
	}
}

// defaultScrollConfig 返回默认配置
func defaultScrollConfig() ScrollConfig {
	return ScrollConfig{
		Size:       1000,
		ScrollTime: 5 * time.Minute,
		Timeout:    30 * time.Second,
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

// ScrollAll 执行完整的滚动查询，将结果填充到dest中
// index: 索引名称，必填
// dest: 必须是指向切片的指针，例如: &[]User{}
// queryBody: 查询的JSON字符串
// 注意：如要限制总结果数，请在queryBody中使用"size"参数，而不是WithSize选项
// WithSize选项只控制每次滚动返回的批次大小
func (c *ScrollSearch) ScrollAll(ctx context.Context, index string, queryBody string, dest interface{}, opts ...ScrollOption) error {
	// 验证索引名称
	if index == "" {
		return fmt.Errorf("索引名称不能为空")
	}

	// 应用配置选项
	config := defaultScrollConfig()
	for _, opt := range opts {
		opt(&config)
	}

	// 验证dest参数
	destValue := reflect.ValueOf(dest)
	if destValue.Kind() != reflect.Ptr {
		return fmt.Errorf("dest必须是指向切片的指针")
	}

	sliceValue := destValue.Elem()
	if sliceValue.Kind() != reflect.Slice {
		return fmt.Errorf("dest必须是指向切片的指针")
	}

	// 获取切片元素类型
	elemType := sliceValue.Type().Elem()

	// 执行初始搜索
	result, err := c.initialSearch(ctx, index, queryBody, config)
	if err != nil {
		return fmt.Errorf("初始搜索失败: %w", err)
	}

	scrollID := result.ScrollID
	fmt.Printf("初始查询返回 %d 条记录，scrollID: %s\n", len(result.Hits.Hits), scrollID)

	defer func() {
		// 清理滚动上下文
		if scrollID != "" {
			c.clearScroll(context.Background(), scrollID)
			fmt.Println("已清理滚动上下文")
		}
	}()

	totalProcessed := 0

	// 处理初始结果
	if len(result.Hits.Hits) > 0 {
		if err := c.appendHitsToSlice(result.Hits.Hits, sliceValue, elemType); err != nil {
			return fmt.Errorf("转换初始结果失败: %w", err)
		}
		totalProcessed += len(result.Hits.Hits)
		fmt.Printf("已处理 %d 条记录\n", totalProcessed)
	}

	// 继续滚动查询，直到没有更多数据
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

		// 更新scrollID（每次scroll请求后ES可能返回新的scrollID）
		if result.ScrollID != "" {
			scrollID = result.ScrollID
		}

		fmt.Printf("滚动查询返回 %d 条记录，scrollID: %s\n", len(result.Hits.Hits), scrollID)

		if len(result.Hits.Hits) > 0 {
			if err := c.appendHitsToSlice(result.Hits.Hits, sliceValue, elemType); err != nil {
				return fmt.Errorf("转换滚动结果失败: %w", err)
			}
			totalProcessed += len(result.Hits.Hits)
			fmt.Printf("已处理 %d 条记录\n", totalProcessed)
		}
	}

	fmt.Printf("滚动查询完成，总共处理 %d 条记录\n", totalProcessed)
	return nil
}

// appendHitsToSlice 将hits追加到目标切片中
func (c *ScrollSearch) appendHitsToSlice(hits []json.RawMessage, sliceValue reflect.Value, elemType reflect.Type) error {
	for _, hit := range hits {
		// 解析hit结构，提取_source
		var hitDoc struct {
			Source json.RawMessage `json:"_source"`
			ID     string          `json:"_id"`
			Index  string          `json:"_index"`
			Type   string          `json:"_type"`
		}

		if err := json.Unmarshal(hit, &hitDoc); err != nil {
			return fmt.Errorf("解析hit失败: %w", err)
		}

		// 创建目标类型的新实例
		newItem := reflect.New(elemType)

		// 将_source数据解析到新实例中
		if err := json.Unmarshal(hitDoc.Source, newItem.Interface()); err != nil {
			return fmt.Errorf("转换为目标类型失败: %w", err)
		}

		// 追加到切片中（解引用指针）
		sliceValue.Set(reflect.Append(sliceValue, newItem.Elem()))
	}

	return nil
}

// initialSearch 执行初始搜索
func (c *ScrollSearch) initialSearch(ctx context.Context, index string, queryBody string, config ScrollConfig) (*scrollResult, error) {

	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

	res, err := c.client.Search(
		c.client.Search.WithContext(ctx),
		c.client.Search.WithIndex(index),
		c.client.Search.WithBody(strings.NewReader(queryBody)),
		c.client.Search.WithScroll(config.ScrollTime),
		c.client.Search.WithSize(config.Size),
	)
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

// continueScroll 继续滚动查询
func (c *ScrollSearch) continueScroll(ctx context.Context, scrollID string, config ScrollConfig) (*scrollResult, error) {

	ctx, cancel := context.WithTimeout(ctx, config.Timeout)
	defer cancel()

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

// clearScroll 清理滚动上下文
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
