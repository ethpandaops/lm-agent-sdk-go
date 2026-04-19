package config

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/hook"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/message"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/observability"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/permission"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/userinput"
)

// Effort controls thinking depth.
type Effort string

const (
	EffortLow    Effort = "low"
	EffortMedium Effort = "medium"
	EffortHigh   Effort = "high"
	EffortMax    Effort = "max"
)

// ThinkingConfig is a marker interface for thinking settings.
type ThinkingConfig interface{ thinkingConfig() }

// ThinkingConfigAdaptive enables adaptive thinking.
type ThinkingConfigAdaptive struct{}

func (ThinkingConfigAdaptive) thinkingConfig() {}

// ThinkingConfigEnabled enables thinking with a token budget.
type ThinkingConfigEnabled struct {
	BudgetTokens int
}

func (ThinkingConfigEnabled) thinkingConfig() {}

// ThinkingConfigDisabled disables thinking.
type ThinkingConfigDisabled struct{}

func (ThinkingConfigDisabled) thinkingConfig() {}

// SessionMetricsRecorder is the narrow observability interface used by the SDK runtime.
// When configured via WithMeterProvider or WithTracerProvider, the SDK creates a recorder
// that emits OpenTelemetry metrics and traces at existing observation points.
// The context parameter enables trace correlation and exemplar propagation.
type SessionMetricsRecorder interface {
	Observe(ctx context.Context, msg message.Message)
}

// QueryLifecycleNotifier is optionally implemented by SessionMetricsRecorder
// implementations that need query lifecycle notifications for TTFT tracking.
type QueryLifecycleNotifier interface {
	MarkQueryStart()
}

// Options contains all SDK options.
type Options struct {
	Logger             *slog.Logger
	SystemPrompt       string
	SystemPromptPreset *SystemPromptPreset
	Model              string
	PermissionMode     string
	MaxTurns           int
	Cwd                string
	User               string

	Hooks                  map[hook.Event][]*hook.Matcher
	Thinking               ThinkingConfig
	Effort                 *Effort
	IncludePartialMessages bool
	MaxBudgetUSD           *float64

	MCPServers map[string]mcp.ServerConfig
	MCPConfig  string

	Tools           ToolsConfig
	AllowedTools    []string
	DisallowedTools []string
	CanUseTool      permission.Callback
	OnUserInput     userinput.Callback

	Resume           string
	ForkSession      bool
	SessionStorePath string

	FallbackModel            string
	PermissionPromptToolName string
	Plugins                  []*PluginConfig
	OutputFormat             map[string]any
	EnableFileCheckpointing  bool
	Transport                Transport
	// Observability
	MeterProvider        metric.MeterProvider
	TracerProvider       trace.TracerProvider
	PrometheusRegisterer prometheus.Registerer

	// MetricsRecorder is the internal observability recorder created from OTel providers.
	// This field is set by the SDK at runtime; users should not set it directly.
	MetricsRecorder SessionMetricsRecorder

	// Observer is the shared observability helper used for SDK-level span and
	// duration instrumentation beyond message-based recording (hook dispatch,
	// explicit tool spans, etc.). Set by the SDK at runtime alongside
	// MetricsRecorder; consumers should not set this directly.
	Observer *observability.Observer

	// LM Studio transport
	APIKey            string
	BaseURL           string
	HTTPReferer       string
	XTitle            string
	RequestTimeout    *time.Duration
	MaxToolIterations int

	// Chat request fields
	Temperature        *float64
	TopP               *float64
	TopK               *float64
	MaxTokens          *int
	PresencePenalty    *float64
	FrequencyPenalty   *float64
	Seed               *int64
	Stop               []string
	Logprobs           *bool
	TopLogprobs        *int
	ParallelToolCalls  *bool
	ToolChoice         any
	ForcedTool         string
	Reasoning          map[string]any
	MaxToolCalls       *int
	StreamIncludeUsage *bool
	Extra              map[string]any

	// LM Studio-specific extensions
	TTL           *time.Duration
	DraftModel    string
	MinP          *float64
	RepeatPenalty *float64
}

// DefaultBaseURL is the default LM Studio OpenAI-compatible base URL.
const DefaultBaseURL = "http://127.0.0.1:1234/v1"

// ApplyDefaults fills missing option defaults.
func (o *Options) ApplyDefaults() {
	if o.PermissionMode == "" {
		o.PermissionMode = string(permission.ModeDefault)
	}
	if o.BaseURL == "" {
		if value := strings.TrimSpace(os.Getenv("LM_BASE_URL")); value != "" {
			o.BaseURL = value
		} else {
			o.BaseURL = DefaultBaseURL
		}
	}
	if o.Model == "" {
		if value := strings.TrimSpace(os.Getenv("LM_MODEL")); value != "" {
			o.Model = value
		}
	}
	if o.MaxToolIterations == 0 {
		o.MaxToolIterations = 8
	}
	if o.StreamIncludeUsage == nil {
		t := true
		o.StreamIncludeUsage = &t
	}
}
