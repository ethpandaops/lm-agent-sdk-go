package lmsdk

import (
	"log/slog"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

// Option configures AgentOptions using the functional options pattern.
// This is the primary option type for configuring clients and queries.
type Option func(*AgentOptions)

// applyAgentOptions applies functional options to an AgentOptions struct.
func applyAgentOptions(opts []Option) *AgentOptions {
	options := &AgentOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return options
}

// ===== Basic Configuration =====

// WithLogger sets the logger for debug output.
// If not set, logging is disabled (silent operation).
func WithLogger(logger *slog.Logger) Option {
	return func(o *AgentOptions) {
		o.Logger = logger
	}
}

// WithSystemPrompt sets the system message to send to the model.
func WithSystemPrompt(prompt string) Option {
	return func(o *AgentOptions) {
		o.SystemPrompt = prompt
	}
}

// WithSystemPromptPreset sets a preset system prompt configuration.
// If set, this takes precedence over WithSystemPrompt.
func WithSystemPromptPreset(preset *SystemPromptPreset) Option {
	return func(o *AgentOptions) {
		o.SystemPromptPreset = preset
	}
}

// WithModel specifies which LM Studio model to use.
func WithModel(model string) Option {
	return func(o *AgentOptions) {
		o.Model = model
	}
}

// WithPermissionMode controls how permissions are handled.
// Valid values: "default", "acceptEdits", "plan", "bypassPermissions".
func WithPermissionMode(mode string) Option {
	return func(o *AgentOptions) {
		o.PermissionMode = mode
	}
}

// WithMaxTurns limits the maximum number of conversation turns.
func WithMaxTurns(maxTurns int) Option {
	return func(o *AgentOptions) {
		o.MaxTurns = maxTurns
	}
}

// WithCwd sets the working directory used by local session and tool features.
func WithCwd(cwd string) Option {
	return func(o *AgentOptions) {
		o.Cwd = cwd
	}
}

// WithUser sets a user identifier for tracking purposes.
func WithUser(user string) Option {
	return func(o *AgentOptions) {
		o.User = user
	}
}

// ===== Hooks =====

// WithHooks configures event hooks for tool interception.
func WithHooks(hooks map[HookEvent][]*HookMatcher) Option {
	return func(o *AgentOptions) {
		o.Hooks = hooks
	}
}

// ===== Token/Budget =====

// WithThinking sets the thinking configuration.
func WithThinking(thinking config.ThinkingConfig) Option {
	return func(o *AgentOptions) {
		o.Thinking = thinking
	}
}

// WithEffort sets the thinking effort level.
func WithEffort(effort config.Effort) Option {
	return func(o *AgentOptions) {
		o.Effort = &effort
	}
}

// WithIncludePartialMessages enables streaming of partial message updates.
func WithIncludePartialMessages(include bool) Option {
	return func(o *AgentOptions) {
		o.IncludePartialMessages = include
	}
}

// WithMaxBudgetUSD sets a cost limit for the session in USD.
func WithMaxBudgetUSD(budget float64) Option {
	return func(o *AgentOptions) {
		o.MaxBudgetUSD = &budget
	}
}

// ===== MCP =====

// WithMCPServers configures external MCP servers to connect to.
// Map key is the server name, value is the server configuration.
func WithMCPServers(servers map[string]MCPServerConfig) Option {
	return func(o *AgentOptions) {
		o.MCPServers = servers
	}
}

// WithMCPConfig sets a path to an MCP config file or a raw JSON string.
// If set, this takes precedence over WithMCPServers.
func WithMCPConfig(config string) Option {
	return func(o *AgentOptions) {
		o.MCPConfig = config
	}
}

// ===== Tools =====

// WithTools specifies which tools are available.
// Accepts ToolsList (tool names) or *ToolsPreset.
func WithTools(tools config.ToolsConfig) Option {
	return func(o *AgentOptions) {
		o.Tools = tools
	}
}

// WithAllowedTools sets pre-approved tools that can be used without prompting.
func WithAllowedTools(tools ...string) Option {
	return func(o *AgentOptions) {
		o.AllowedTools = tools
	}
}

// WithDisallowedTools sets tools that are explicitly blocked.
func WithDisallowedTools(tools ...string) Option {
	return func(o *AgentOptions) {
		o.DisallowedTools = tools
	}
}

// WithCanUseTool sets a callback for permission checking before each tool use.
func WithCanUseTool(callback ToolPermissionCallback) Option {
	return func(o *AgentOptions) {
		o.CanUseTool = callback
	}
}

// WithOnUserInput sets a callback for handling SDK user-input tool prompts.
func WithOnUserInput(callback UserInputCallback) Option {
	return func(o *AgentOptions) {
		o.OnUserInput = callback
	}
}

// ===== Session =====

// WithResume sets a session ID to resume from.
func WithResume(sessionID string) Option {
	return func(o *AgentOptions) {
		o.Resume = sessionID
	}
}

// WithForkSession indicates whether to fork the resumed session to a new ID.
func WithForkSession(fork bool) Option {
	return func(o *AgentOptions) {
		o.ForkSession = fork
	}
}

// WithSessionStorePath enables durable session persistence at a JSON file path.
// When set, resume/fork state can survive process restarts.
func WithSessionStorePath(path string) Option {
	return func(o *AgentOptions) {
		o.SessionStorePath = path
	}
}

// ===== Advanced =====

// WithFallbackModel specifies a model to use if the primary model fails.
func WithFallbackModel(model string) Option {
	return func(o *AgentOptions) {
		o.FallbackModel = model
	}
}

// WithPermissionPromptToolName specifies the tool name to use for permission prompts.
func WithPermissionPromptToolName(name string) Option {
	return func(o *AgentOptions) {
		o.PermissionPromptToolName = name
	}
}

// WithPlugins configures plugins to load.
func WithPlugins(plugins ...*SdkPluginConfig) Option {
	return func(o *AgentOptions) {
		o.Plugins = plugins
	}
}

// WithOutputFormat specifies a JSON schema for structured output.
//
// The canonical format uses a wrapper object:
//
//	lmsdk.WithOutputFormat(map[string]any{
//	    "type": "json_schema",
//	    "schema": map[string]any{
//	        "type":       "object",
//	        "properties": map[string]any{...},
//	        "required":   []string{...},
//	    },
//	})
//
// Raw JSON schemas (without the wrapper) are also accepted and auto-wrapped:
//
//	lmsdk.WithOutputFormat(map[string]any{
//	    "type":       "object",
//	    "properties": map[string]any{...},
//	    "required":   []string{...},
//	})
//
// Structured output is available on [ResultMessage].StructuredOutput (parsed)
// or [ResultMessage].Result (JSON string).
func WithOutputFormat(format map[string]any) Option {
	return func(o *AgentOptions) {
		o.OutputFormat = format
	}
}

// WithEnableFileCheckpointing enables file change tracking and rewinding.
func WithEnableFileCheckpointing(enable bool) Option {
	return func(o *AgentOptions) {
		o.EnableFileCheckpointing = enable
	}
}

// WithSDKTools registers high-level Tool instances as an in-process MCP server.
// Tools are exposed under the "sdk" MCP server name (tool names: mcp__sdk__<name>).
// Each tool is automatically added to AllowedTools.
func WithSDKTools(tools ...Tool) Option {
	return func(o *AgentOptions) {
		if len(tools) == 0 {
			return
		}

		if o.MCPServers == nil {
			o.MCPServers = make(map[string]MCPServerConfig, 1)
		}
		o.MCPServers["sdk"] = createSDKToolServer(tools)
		for _, t := range tools {
			o.AllowedTools = append(o.AllowedTools, "mcp__sdk__"+t.Name())
		}
	}
}

// WithTransport injects a custom transport implementation.
// The transport must implement the Transport interface.
func WithTransport(transport Transport) Option {
	return func(o *AgentOptions) {
		o.Transport = transport
	}
}

// WithAPIKey sets the LM Studio API key directly.
func WithAPIKey(apiKey string) Option {
	return func(o *AgentOptions) {
		o.APIKey = apiKey
	}
}

// WithBaseURL overrides the LM Studio base URL.
func WithBaseURL(baseURL string) Option {
	return func(o *AgentOptions) {
		o.BaseURL = baseURL
	}
}

// WithHTTPReferer sets the HTTP-Referer header on outgoing requests.
func WithHTTPReferer(referer string) Option {
	return func(o *AgentOptions) {
		o.HTTPReferer = referer
	}
}

// WithXTitle sets the X-Title header on outgoing requests.
func WithXTitle(title string) Option {
	return func(o *AgentOptions) {
		o.XTitle = title
	}
}

// WithRequestTimeout sets HTTP request timeout for LM Studio calls.
func WithRequestTimeout(timeout time.Duration) Option {
	return func(o *AgentOptions) {
		o.RequestTimeout = &timeout
	}
}

// WithMaxToolIterations sets maximum tool-call loops per query.
func WithMaxToolIterations(max int) Option {
	return func(o *AgentOptions) {
		o.MaxToolIterations = max
	}
}

// ===== Sampling =====

// WithTemperature sets sampling temperature.
func WithTemperature(temperature float64) Option {
	return func(o *AgentOptions) {
		o.Temperature = &temperature
	}
}

// WithMaxTokens sets chat max_tokens.
func WithMaxTokens(max int) Option {
	return func(o *AgentOptions) {
		o.MaxTokens = &max
	}
}

// WithTopP sets nucleus sampling probability.
func WithTopP(topP float64) Option {
	return func(o *AgentOptions) {
		o.TopP = &topP
	}
}

// WithTopK sets top-k sampling.
func WithTopK(topK float64) Option {
	return func(o *AgentOptions) {
		o.TopK = &topK
	}
}

// WithMinP sets the llama.cpp min_p sampling parameter.
func WithMinP(minP float64) Option {
	return func(o *AgentOptions) {
		o.MinP = &minP
	}
}

// WithRepeatPenalty sets the llama.cpp repeat_penalty parameter.
func WithRepeatPenalty(penalty float64) Option {
	return func(o *AgentOptions) {
		o.RepeatPenalty = &penalty
	}
}

// WithPresencePenalty sets presence penalty.
func WithPresencePenalty(v float64) Option {
	return func(o *AgentOptions) {
		o.PresencePenalty = &v
	}
}

// WithFrequencyPenalty sets frequency penalty.
func WithFrequencyPenalty(v float64) Option {
	return func(o *AgentOptions) {
		o.FrequencyPenalty = &v
	}
}

// WithSeed sets deterministic seed where supported.
func WithSeed(seed int64) Option {
	return func(o *AgentOptions) {
		o.Seed = &seed
	}
}

// WithStop sets stop sequences.
func WithStop(stop ...string) Option {
	return func(o *AgentOptions) {
		o.Stop = append([]string(nil), stop...)
	}
}

// WithLogprobs enables token log probabilities where supported.
func WithLogprobs(enable bool) Option {
	return func(o *AgentOptions) {
		o.Logprobs = &enable
	}
}

// WithTopLogprobs sets top_logprobs where supported.
func WithTopLogprobs(v int) Option {
	return func(o *AgentOptions) {
		o.TopLogprobs = &v
	}
}

// WithParallelToolCalls sets parallel_tool_calls.
func WithParallelToolCalls(enable bool) Option {
	return func(o *AgentOptions) {
		o.ParallelToolCalls = &enable
	}
}

// WithToolChoice sets the tool_choice payload.
//
// LM Studio only accepts the string forms "none", "auto", and "required" —
// the OpenAI object form `{"type": "function", "function": {"name": ...}}`
// is rejected with HTTP 400. Use [WithForceTool] to force a specific
// function instead.
func WithToolChoice(choice any) Option {
	return func(o *AgentOptions) {
		o.ToolChoice = choice
	}
}

// WithForceTool forces the model to invoke a specific tool on the next turn.
// This is the LM-Studio-supported workaround for OpenAI's object-form
// tool_choice (see [WithToolChoice]): the SDK sets `tool_choice: "required"`
// and filters `tools[]` down to just the named tool for the outgoing
// request, so the model has no other function to call.
//
// The tool must already be registered via [WithSDKTools], [WithMCPServers],
// or [WithMCPConfig]. Pass the full public name (e.g. `"mcp__sdk__add"`).
// Pass an empty string to clear a previously-set force.
func WithForceTool(name string) Option {
	return func(o *AgentOptions) {
		o.ForcedTool = name
	}
}

// WithReasoning sets reasoning configuration passed through as the `reasoning` field.
func WithReasoning(reasoning map[string]any) Option {
	return func(o *AgentOptions) {
		o.Reasoning = reasoning
	}
}

// WithMaxToolCalls sets the max_tool_calls field on outgoing requests.
func WithMaxToolCalls(max int) Option {
	return func(o *AgentOptions) {
		o.MaxToolCalls = &max
	}
}

// WithExtra merges raw request fields into the outgoing payload.
// Use for fields not covered by dedicated With* options.
func WithExtra(extra map[string]any) Option {
	return func(o *AgentOptions) {
		o.Extra = extra
	}
}

// ===== LM Studio-specific =====

// WithTTL sets the `ttl` field (in seconds) controlling how long a JIT-loaded
// model stays in memory after the request. See LM Studio's JIT and
// Auto-Evict documentation for details.
func WithTTL(ttl time.Duration) Option {
	return func(o *AgentOptions) {
		o.TTL = &ttl
	}
}

// WithDraftModel sets the `draft_model` field enabling speculative decoding.
// The draft model must share the same vocabulary/tokenizer family as the
// primary model. See LM Studio's Speculative Decoding docs.
func WithDraftModel(model string) Option {
	return func(o *AgentOptions) {
		o.DraftModel = model
	}
}

// WithStreamUsage controls whether stream_options.include_usage is set on
// streaming requests. Defaults to true; set to false to opt out.
func WithStreamUsage(enable bool) Option {
	return func(o *AgentOptions) {
		o.StreamIncludeUsage = &enable
	}
}

// ===== Observability =====

// WithMeterProvider sets an OpenTelemetry MeterProvider for recording SDK
// metrics. When nil (the default) all metric recording is a noop.
func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(o *AgentOptions) {
		o.MeterProvider = mp
	}
}

// WithTracerProvider sets an OpenTelemetry TracerProvider for recording SDK
// spans. When nil (the default) all span creation is a noop.
func WithTracerProvider(tp trace.TracerProvider) Option {
	return func(o *AgentOptions) {
		o.TracerProvider = tp
	}
}

// WithPrometheusRegisterer configures the OTel-to-Prometheus bridge.
// When set, the SDK automatically creates a Prometheus-backed MeterProvider.
// If a MeterProvider is also set via WithMeterProvider, it takes precedence.
func WithPrometheusRegisterer(reg prometheus.Registerer) Option {
	return func(o *AgentOptions) {
		o.PrometheusRegisterer = reg
	}
}
