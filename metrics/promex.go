package promex

import (
	"github.com/prometheus/client_golang/prometheus"
)

var std *prometheus.Registry

func init() {
	std = prometheus.NewRegistry()

}

// Register prometheus collector
func Register(c prometheus.Collector) error {
	return std.Register(c)
}
