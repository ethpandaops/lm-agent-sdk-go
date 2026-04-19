package lmsdk

import (
	"github.com/ethpandaops/lm-agent-sdk-go/internal/observability"
	"github.com/prometheus/client_golang/prometheus"
)

// classifyErrorForTest exposes the internal Observer error classification for test assertions.
func classifyErrorForTest(err error) string {
	obs := observability.Noop()
	return string(obs.Classify(err))
}

// statusClassForTest exposes the internal StatusClassOf for test assertions.
func statusClassForTest(code int) string {
	return observability.StatusClassOf(code)
}

// newTestPrometheusRegistry creates a prometheus.Registry for test use.
// prometheus.Registry implements both prometheus.Registerer and prometheus.Gatherer.
func newTestPrometheusRegistry() *prometheus.Registry {
	return prometheus.NewRegistry()
}
