package metrics

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/Educentr/go-project-starter-runtime/pkg/ds"
)

// selfCollector implements Collector for a single Metric so that the Metric
// collects itself. Add it as an anonymous field to a struct that implements
// Metric, and call init with the Metric itself as an argument.
type infoCollector struct {
	self prometheus.Metric
}

// Describe implements Collector.
func (c *infoCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.self.Desc()
}

// Collect implements Collector.
func (c *infoCollector) Collect(ch chan<- prometheus.Metric) {
	ch <- c.self
}

func InfoCollector(desc *prometheus.Desc) prometheus.Collector {
	return &infoCollector{
		self: prometheus.MustNewConstMetric(
			desc,
			prometheus.GaugeValue,
			1,
		),
	}
}

// BuildInfoCollector creates a Prometheus collector that exposes build information.
// The serviceName parameter is the service-level name (Main.Name, e.g., "my-api").
// The info.AppName is the application name within the service (Application.Name, e.g., "web-app").
func BuildInfoCollector(serviceName string, info *ds.AppInfo) prometheus.Collector {
	return InfoCollector(
		prometheus.NewDesc(
			serviceName+"_build_info",
			"Build information about the "+serviceName+" service.",
			nil,
			prometheus.Labels{
				"service_name": serviceName,
				"app_name":     info.AppName,
				"version":      info.Version,
				"build_commit": info.BuildCommit,
				"build_time":   info.BuildTime,
			},
		),
	)
}
