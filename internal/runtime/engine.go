package runtime

import (
	agenterrclass "github.com/ethpandaops/agent-sdk-observability/errclass"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/controlplane"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/observability"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/session"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/lmstudio"
)

// Engine owns runtime dependencies used by Query and Client.
type Engine struct {
	runner *QueryRunner
}

// NewEngine creates a new runtime engine.
func NewEngine(opts *config.Options, sessions *session.Manager) *Engine {
	if opts.Transport == nil {
		opts.Transport = lmstudio.NewHTTPTransport(opts)
	}

	obs := resolveObserver(opts)
	// Both are expected control-flow conditions, mapped to InvalidRequest to
	// keep the error.type label set closed.
	obs.RegisterSentinel(session.ErrNoCheckpoint, agenterrclass.InvalidRequest)
	obs.RegisterSentinel(controlplane.ErrUnsupportedControl, agenterrclass.InvalidRequest)

	// Inject observer into the HTTP transport if it's the default type.
	if ht, ok := opts.Transport.(*lmstudio.HTTPTransport); ok {
		ht.SetObserver(obs)
	}

	// Inject observer into the session manager.
	if sessions != nil {
		sessions.SetObserver(obs)
	}

	return &Engine{runner: NewQueryRunner(opts, sessions, obs)}
}

// Runner returns the query runner.
func (e *Engine) Runner() *QueryRunner { return e.runner }

// resolveObserver returns the Observer set by initMetricsRecorder (via
// otel.go) when OTel providers were configured. Falls back to a noop
// Observer so NewEngine stays infallible.
func resolveObserver(opts *config.Options) *observability.Observer {
	if opts != nil && opts.Observer != nil {
		return opts.Observer
	}

	return observability.Noop()
}
