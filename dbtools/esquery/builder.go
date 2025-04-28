package esquery

import (
	jsoniter "github.com/json-iterator/go"
)

type Map = map[string]interface{}

// Builder ES DSL构造器
type Builder struct {
	body Map
}

// NewBuilder 创建新的DSL构造器
func NewBuilder() *Builder {
	return &Builder{
		body: make(map[string]interface{}),
	}
}

// set 通用设置方法，支持链式调用
func (b *Builder) set(key string, value interface{}) *Builder {
	b.body[key] = value
	return b
}

// SetQuery 设置查询条件
func (b *Builder) SetQuery(query interface{}) *Builder {
	return b.set("query", query)
}

// SetAggs 设置聚合
func (b *Builder) SetAggs(aggs interface{}) *Builder {
	return b.set("aggs", aggs)
}

// SetSort 设置排序
func (b *Builder) SetSort(sort interface{}) *Builder {
	return b.set("sort", sort)
}

// SetSize 设置返回数量
func (b *Builder) SetSize(size int) *Builder {
	return b.set("size", size)
}

// SetFrom 设置偏移量
func (b *Builder) SetFrom(from int) *Builder {
	return b.set("from", from)
}

// SetSource 设置返回字段
func (b *Builder) SetSource(fields []string) *Builder {
	return b.set("_source", fields)
}

// SetHighlight 设置高亮
func (b *Builder) SetHighlight(highlight interface{}) *Builder {
	return b.set("highlight", highlight)
}

// Build 构建DSL
func (b *Builder) Build() map[string]interface{} {
	return b.body
}

// BuildBytes 构建并返回 []byte
func (b *Builder) BuildBytes() ([]byte, error) {
	return jsoniter.Marshal(b.body)
}

func BuildSortField(field string, order string) Map {
	return BuildMap(field, BuildMap("order", order))
}

func BuildSortScore(order string) Map {
	return BuildMap("_score", BuildMap("order", order))
}

type HighlightCfg struct {
	fragmentSize      int
	numberOfFragments int
	PreTags           []string
	PostTags          []string
}

type HighlightOption func(*HighlightCfg)

func WithFragmentSize(size int) HighlightOption {
	return func(cfg *HighlightCfg) {
		cfg.fragmentSize = size
	}
}

func WithNumberOfFragments(number int) HighlightOption {
	return func(cfg *HighlightCfg) {
		cfg.numberOfFragments = number
	}
}

func WithPreTags(tags []string) HighlightOption {
	return func(cfg *HighlightCfg) {
		cfg.PreTags = tags
	}
}

func WithPostTags(tags []string) HighlightOption {
	return func(cfg *HighlightCfg) {
		cfg.PostTags = tags
	}
}

func BuildHighlightField(fields []string, options ...HighlightOption) Map {
	cfg := &HighlightCfg{
		fragmentSize:      1500,
		numberOfFragments: 5,
		PreTags:           []string{"<em>"},
		PostTags:          []string{"</em>"},
	}
	for _, option := range options {
		option(cfg)
	}

	fieldMap := make(map[string]interface{})
	for _, field := range fields {
		fieldMap[field] = Map{}
	}
	return BuildMap("fields", fieldMap,
		"fragment_size", cfg.fragmentSize,
		"number_of_fragments", cfg.numberOfFragments,
		"pre_tags", cfg.PreTags,
		"post_tags", cfg.PostTags,
	)
}
