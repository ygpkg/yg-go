package metrics

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// EasyMetrics 便捷的指标管理器
type EasyMetrics struct {
	registry   *prometheus.Registry
	counters   map[string]*prometheus.CounterVec
	gauges     map[string]*prometheus.GaugeVec
	histograms map[string]*prometheus.HistogramVec
	summaries  map[string]*prometheus.SummaryVec
	labelKeys  map[string][]string
	mu         sync.RWMutex
	namespace  string
	subsystem  string
}

// Options 配置选项
type Options struct {
	Namespace  string
	Subsystem  string
	Buckets    []float64
	Objectives map[float64]float64
}

// NewEasyMetrics 创建新的便捷指标管理器
func NewEasyMetrics(registry *prometheus.Registry, opts ...Options) *EasyMetrics {
	em := &EasyMetrics{
		registry:   std,
		counters:   make(map[string]*prometheus.CounterVec),
		gauges:     make(map[string]*prometheus.GaugeVec),
		histograms: make(map[string]*prometheus.HistogramVec),
		summaries:  make(map[string]*prometheus.SummaryVec),
		labelKeys:  make(map[string][]string),
	}

	if len(opts) > 0 {
		em.namespace = opts[0].Namespace
		em.subsystem = opts[0].Subsystem
	}

	return em
}

// extractLabelKeys 从labels中提取并排序key
func (em *EasyMetrics) extractLabelKeys(labels prometheus.Labels) []string {
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys) // 确保顺序一致
	return keys
}

// getLabelKey 生成用于存储的key
func (em *EasyMetrics) getLabelKey(name string, keys []string) string {
	return fmt.Sprintf("%s:%s", name, strings.Join(keys, ","))
}

// autoRegisterCounter 自动注册Counter
func (em *EasyMetrics) autoRegisterCounter(name string, labels prometheus.Labels) (*prometheus.CounterVec, error) {
	labelKeys := em.extractLabelKeys(labels)
	key := em.getLabelKey(name, labelKeys)

	if counter, exists := em.counters[key]; exists {
		return counter, nil
	}

	counter := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: em.namespace,
			Subsystem: em.subsystem,
			Name:      name,
			Help:      fmt.Sprintf("Auto-generated counter metric: %s", name),
		},
		labelKeys,
	)

	if err := em.registry.Register(counter); err != nil {
		return nil, fmt.Errorf("failed to register counter %s: %w", name, err)
	}

	em.counters[key] = counter
	em.labelKeys[key] = labelKeys
	return counter, nil
}

// autoRegisterGauge 自动注册Gauge
func (em *EasyMetrics) autoRegisterGauge(name string, labels prometheus.Labels) (*prometheus.GaugeVec, error) {
	labelKeys := em.extractLabelKeys(labels)
	key := em.getLabelKey(name, labelKeys)

	if gauge, exists := em.gauges[key]; exists {
		return gauge, nil
	}

	gauge := prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: em.namespace,
			Subsystem: em.subsystem,
			Name:      name,
			Help:      fmt.Sprintf("Auto-generated gauge metric: %s", name),
		},
		labelKeys,
	)

	if err := em.registry.Register(gauge); err != nil {
		return nil, fmt.Errorf("failed to register gauge %s: %w", name, err)
	}

	em.gauges[key] = gauge
	em.labelKeys[key] = labelKeys
	return gauge, nil
}

// autoRegisterHistogram 自动注册Histogram
func (em *EasyMetrics) autoRegisterHistogram(name string, labels prometheus.Labels, buckets []float64) (*prometheus.HistogramVec, error) {
	labelKeys := em.extractLabelKeys(labels)
	key := em.getLabelKey(name, labelKeys)

	if histogram, exists := em.histograms[key]; exists {
		return histogram, nil
	}

	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}

	histogram := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: em.namespace,
			Subsystem: em.subsystem,
			Name:      name,
			Help:      fmt.Sprintf("Auto-generated histogram metric: %s", name),
			Buckets:   buckets,
		},
		labelKeys,
	)

	if err := em.registry.Register(histogram); err != nil {
		return nil, fmt.Errorf("failed to register histogram %s: %w", name, err)
	}

	em.histograms[key] = histogram
	em.labelKeys[key] = labelKeys
	return histogram, nil
}

// autoRegisterSummary 自动注册Summary
func (em *EasyMetrics) autoRegisterSummary(name string, labels prometheus.Labels, objectives map[float64]float64) (*prometheus.SummaryVec, error) {
	labelKeys := em.extractLabelKeys(labels)
	key := em.getLabelKey(name, labelKeys)

	if summary, exists := em.summaries[key]; exists {
		return summary, nil
	}

	if len(objectives) == 0 {
		objectives = map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001}
	}

	summary := prometheus.NewSummaryVec(
		prometheus.SummaryOpts{
			Namespace:  em.namespace,
			Subsystem:  em.subsystem,
			Name:       name,
			Help:       fmt.Sprintf("Auto-generated summary metric: %s", name),
			Objectives: objectives,
		},
		labelKeys,
	)

	if err := em.registry.Register(summary); err != nil {
		return nil, fmt.Errorf("failed to register summary %s: %w", name, err)
	}

	em.summaries[key] = summary
	em.labelKeys[key] = labelKeys
	return summary, nil
}

// Counter 便捷的Counter操作
func (em *EasyMetrics) Counter(name string) *CounterBuilder {
	return &CounterBuilder{
		em:   em,
		name: name,
	}
}

// Gauge 便捷的Gauge操作
func (em *EasyMetrics) Gauge(name string) *GaugeBuilder {
	return &GaugeBuilder{
		em:   em,
		name: name,
	}
}

// Histogram 便捷的Histogram操作
func (em *EasyMetrics) Histogram(name string) *HistogramBuilder {
	return &HistogramBuilder{
		em:   em,
		name: name,
	}
}

// Summary 便捷的Summary操作
func (em *EasyMetrics) Summary(name string) *SummaryBuilder {
	return &SummaryBuilder{
		em:   em,
		name: name,
	}
}

// CounterBuilder Counter构建器
type CounterBuilder struct {
	em     *EasyMetrics
	name   string
	labels prometheus.Labels
}

// With 设置labels
func (cb *CounterBuilder) With(labels prometheus.Labels) *CounterBuilder {
	cb.labels = labels
	return cb
}

// Inc 增加计数
func (cb *CounterBuilder) Inc() error {
	cb.em.mu.Lock()
	defer cb.em.mu.Unlock()

	if cb.labels == nil {
		cb.labels = prometheus.Labels{}
	}

	counter, err := cb.em.autoRegisterCounter(cb.name, cb.labels)
	if err != nil {
		return err
	}

	counter.With(cb.labels).Inc()
	return nil
}

// Add 增加指定值
func (cb *CounterBuilder) Add(value float64) error {
	cb.em.mu.Lock()
	defer cb.em.mu.Unlock()

	if cb.labels == nil {
		cb.labels = prometheus.Labels{}
	}

	counter, err := cb.em.autoRegisterCounter(cb.name, cb.labels)
	if err != nil {
		return err
	}

	counter.With(cb.labels).Add(value)
	return nil
}

// GaugeBuilder Gauge构建器
type GaugeBuilder struct {
	em     *EasyMetrics
	name   string
	labels prometheus.Labels
}

// With 设置labels
func (gb *GaugeBuilder) With(labels prometheus.Labels) *GaugeBuilder {
	gb.labels = labels
	return gb
}

// Set 设置值
func (gb *GaugeBuilder) Set(value float64) error {
	gb.em.mu.Lock()
	defer gb.em.mu.Unlock()

	if gb.labels == nil {
		gb.labels = prometheus.Labels{}
	}

	gauge, err := gb.em.autoRegisterGauge(gb.name, gb.labels)
	if err != nil {
		return err
	}

	gauge.With(gb.labels).Set(value)
	return nil
}

// Inc 增加1
func (gb *GaugeBuilder) Inc() error {
	gb.em.mu.Lock()
	defer gb.em.mu.Unlock()

	if gb.labels == nil {
		gb.labels = prometheus.Labels{}
	}

	gauge, err := gb.em.autoRegisterGauge(gb.name, gb.labels)
	if err != nil {
		return err
	}

	gauge.With(gb.labels).Inc()
	return nil
}

// Dec 减少1
func (gb *GaugeBuilder) Dec() error {
	gb.em.mu.Lock()
	defer gb.em.mu.Unlock()

	if gb.labels == nil {
		gb.labels = prometheus.Labels{}
	}

	gauge, err := gb.em.autoRegisterGauge(gb.name, gb.labels)
	if err != nil {
		return err
	}

	gauge.With(gb.labels).Dec()
	return nil
}

// Add 增加指定值
func (gb *GaugeBuilder) Add(value float64) error {
	gb.em.mu.Lock()
	defer gb.em.mu.Unlock()

	if gb.labels == nil {
		gb.labels = prometheus.Labels{}
	}

	gauge, err := gb.em.autoRegisterGauge(gb.name, gb.labels)
	if err != nil {
		return err
	}

	gauge.With(gb.labels).Add(value)
	return nil
}

// HistogramBuilder Histogram构建器
type HistogramBuilder struct {
	em      *EasyMetrics
	name    string
	labels  prometheus.Labels
	buckets []float64
}

// With 设置labels
func (hb *HistogramBuilder) With(labels prometheus.Labels) *HistogramBuilder {
	hb.labels = labels
	return hb
}

// Buckets 设置桶
func (hb *HistogramBuilder) Buckets(buckets ...float64) *HistogramBuilder {
	hb.buckets = buckets
	return hb
}

// Observe 观察值
func (hb *HistogramBuilder) Observe(value float64) error {
	hb.em.mu.Lock()
	defer hb.em.mu.Unlock()

	if hb.labels == nil {
		hb.labels = prometheus.Labels{}
	}

	histogram, err := hb.em.autoRegisterHistogram(hb.name, hb.labels, hb.buckets)
	if err != nil {
		return err
	}

	histogram.With(hb.labels).Observe(value)
	return nil
}

// Time 测量函数执行时间
func (hb *HistogramBuilder) Time(fn func()) error {
	start := time.Now()
	fn()
	duration := time.Since(start).Seconds()
	return hb.Observe(duration)
}

// Timer 返回计时器
func (hb *HistogramBuilder) Timer() *Timer {
	return &Timer{
		start:   time.Now(),
		builder: hb,
	}
}

// SummaryBuilder Summary构建器
type SummaryBuilder struct {
	em         *EasyMetrics
	name       string
	labels     prometheus.Labels
	objectives map[float64]float64
}

// With 设置labels
func (sb *SummaryBuilder) With(labels prometheus.Labels) *SummaryBuilder {
	sb.labels = labels
	return sb
}

// Objectives 设置目标
func (sb *SummaryBuilder) Objectives(objectives map[float64]float64) *SummaryBuilder {
	sb.objectives = objectives
	return sb
}

// Observe 观察值
func (sb *SummaryBuilder) Observe(value float64) error {
	sb.em.mu.Lock()
	defer sb.em.mu.Unlock()

	if sb.labels == nil {
		sb.labels = prometheus.Labels{}
	}

	summary, err := sb.em.autoRegisterSummary(sb.name, sb.labels, sb.objectives)
	if err != nil {
		return err
	}

	summary.With(sb.labels).Observe(value)
	return nil
}

// Time 测量函数执行时间
func (sb *SummaryBuilder) Time(fn func()) error {
	start := time.Now()
	fn()
	duration := time.Since(start).Seconds()
	return sb.Observe(duration)
}

// Timer 计时器
type Timer struct {
	start   time.Time
	builder *HistogramBuilder
}

// Stop 停止计时并记录
func (t *Timer) Stop() error {
	duration := time.Since(t.start).Seconds()
	return t.builder.Observe(duration)
}

// GetHandler 获取HTTP处理器
func (em *EasyMetrics) GetHandler() http.Handler {
	return promhttp.HandlerFor(em.registry, promhttp.HandlerOpts{})
}

// StartServer 启动指标服务器
func (em *EasyMetrics) StartServer(addr string) error {
	http.Handle("/metrics", em.GetHandler())
	return http.ListenAndServe(addr, nil)
}
