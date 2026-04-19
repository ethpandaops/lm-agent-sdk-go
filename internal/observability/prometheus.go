package observability

import (
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/metric"

	"github.com/ethpandaops/agent-sdk-observability/promexporter"
)

func NewPrometheusMeterProvider(reg prometheus.Registerer) (metric.MeterProvider, error) {
	return promexporter.NewMeterProvider(reg)
}
