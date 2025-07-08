package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	std *prometheus.Registry
	em  *EasyMetrics
)

func init() {
	std = prometheus.NewRegistry()
	em = NewEasyMetrics(std)
}

// 全局便捷函数

// Counter 便捷的Counter操作
func Counter(name string) *CounterBuilder {
	return em.Counter(name)
}

// Gauge 便捷的Gauge操作
func Gauge(name string) *GaugeBuilder {
	return em.Gauge(name)
}

// Histogram 便捷的Histogram操作
func Histogram(name string) *HistogramBuilder {
	return em.Histogram(name)
}

// Summary 便捷的Summary操作
func Summary(name string) *SummaryBuilder {
	return em.Summary(name)
}

// GetHandler 获取prometheus的http.Handler
func GetHandler() http.Handler {
	return em.GetHandler()
}

// StartServer 启动prometheus的http服务
func StartServer(addr string) error {
	return em.StartServer(addr)
}

// responseWriter 用于捕获状态码
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}
