package compat

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the compatibility system
type Metrics struct {
	// Counter for workaround activations
	WorkaroundActivations *prometheus.CounterVec

	// Counter for client detections
	ClientDetections *prometheus.CounterVec

	// Counter for HTTP protocol overrides
	HTTPProtocolOverrides *prometheus.CounterVec

	// Counter for manifest fixes applied
	ManifestFixes *prometheus.CounterVec

	// Counter for header workarounds applied
	HeaderWorkarounds *prometheus.CounterVec

	// Histogram for workaround processing time
	WorkaroundDuration *prometheus.HistogramVec

	// Gauge for active workarounds per client version
	ActiveWorkarounds *prometheus.GaugeVec
}

// NewMetrics creates and registers all compatibility metrics
func NewMetrics(prefix string) *Metrics {
	if prefix == "" {
		prefix = "ads_registry_compat_"
	}

	return &Metrics{
		WorkaroundActivations: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "workaround_activations_total",
				Help: "Total number of workaround activations by client and workaround type",
			},
			[]string{"client", "version", "workaround"},
		),

		ClientDetections: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "client_detections_total",
				Help: "Total number of client detections by client type and version",
			},
			[]string{"client", "version", "protocol"},
		),

		HTTPProtocolOverrides: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "http_protocol_overrides_total",
				Help: "Total number of HTTP protocol overrides (HTTP/2 to HTTP/1.1)",
			},
			[]string{"from", "to", "reason"},
		),

		ManifestFixes: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "manifest_fixes_total",
				Help: "Total number of manifest-related fixes applied",
			},
			[]string{"client", "fix_type"},
		),

		HeaderWorkarounds: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: prefix + "header_workarounds_total",
				Help: "Total number of header workarounds applied",
			},
			[]string{"header", "workaround"},
		),

		WorkaroundDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    prefix + "workaround_duration_seconds",
				Help:    "Time spent processing workarounds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"workaround"},
		),

		ActiveWorkarounds: promauto.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: prefix + "active_workarounds",
				Help: "Number of currently active workarounds by client version",
			},
			[]string{"client", "version"},
		),
	}
}

// RecordWorkaroundActivation increments the workaround activation counter
func (m *Metrics) RecordWorkaroundActivation(client, version, workaround string) {
	m.WorkaroundActivations.WithLabelValues(client, version, workaround).Inc()
}

// RecordClientDetection increments the client detection counter
func (m *Metrics) RecordClientDetection(client, version, protocol string) {
	m.ClientDetections.WithLabelValues(client, version, protocol).Inc()
}

// RecordHTTPProtocolOverride increments the protocol override counter
func (m *Metrics) RecordHTTPProtocolOverride(from, to, reason string) {
	m.HTTPProtocolOverrides.WithLabelValues(from, to, reason).Inc()
}

// RecordManifestFix increments the manifest fix counter
func (m *Metrics) RecordManifestFix(client, fixType string) {
	m.ManifestFixes.WithLabelValues(client, fixType).Inc()
}

// RecordHeaderWorkaround increments the header workaround counter
func (m *Metrics) RecordHeaderWorkaround(header, workaround string) {
	m.HeaderWorkarounds.WithLabelValues(header, workaround).Inc()
}

// ObserveWorkaroundDuration records the time spent processing a workaround
func (m *Metrics) ObserveWorkaroundDuration(workaround string, seconds float64) {
	m.WorkaroundDuration.WithLabelValues(workaround).Observe(seconds)
}

// SetActiveWorkarounds sets the gauge for active workarounds
func (m *Metrics) SetActiveWorkarounds(client, version string, count int) {
	m.ActiveWorkarounds.WithLabelValues(client, version).Set(float64(count))
}
