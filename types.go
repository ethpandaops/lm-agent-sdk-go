package lmsdk

import (
	"iter"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/hook"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/message"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/model"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/permission"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/userinput"
)

// Re-export types from internal packages

// ===== Transport =====

// Transport defines the runtime transport surface.
// Re-exported from internal/config for public API access.
// See transport.go for full documentation.
// type Transport = config.Transport (defined in transport.go)

// ===== Options and Configuration =====

// AgentOptions configures SDK and LM Studio request behavior.
type AgentOptions = config.Options

// ===== Thinking Configuration =====

// ThinkingConfig controls extended thinking behavior.
type ThinkingConfig = config.ThinkingConfig

// ThinkingConfigAdaptive enables adaptive thinking mode.
type ThinkingConfigAdaptive = config.ThinkingConfigAdaptive

// ThinkingConfigEnabled enables thinking with a specific token budget.
type ThinkingConfigEnabled = config.ThinkingConfigEnabled

// ThinkingConfigDisabled disables extended thinking.
type ThinkingConfigDisabled = config.ThinkingConfigDisabled

// Effort controls thinking depth.
type Effort = config.Effort

const (
	// EffortLow uses minimal thinking.
	EffortLow = config.EffortLow
	// EffortMedium uses moderate thinking.
	EffortMedium = config.EffortMedium
	// EffortHigh uses deep thinking.
	EffortHigh = config.EffortHigh
	// EffortMax uses maximum thinking depth.
	EffortMax = config.EffortMax
)

// SystemPromptPreset defines a system prompt preset configuration.
type SystemPromptPreset = config.SystemPromptPreset

// SdkPluginConfig configures a plugin to load.
type SdkPluginConfig = config.PluginConfig

// ToolsPreset represents a preset configuration for available tools.
type ToolsPreset = config.ToolsPreset

// ToolsConfig is an interface for configuring available tools.
// It represents either a list of tool names or a preset configuration.
type ToolsConfig = config.ToolsConfig

// ToolsList is a list of tool names to make available.
type ToolsList = config.ToolsList

// ===== Messages =====

// Message represents any message in the conversation.
type Message = message.Message

// UserMessage represents a message from the user.
type UserMessage = message.UserMessage

// UserMessageContent represents content that can be either a string or []ContentBlock.
type UserMessageContent = message.UserMessageContent

// NewUserMessageContent creates UserMessageContent from a string.
var NewUserMessageContent = message.NewUserMessageContent

// NewUserMessageContentBlocks creates UserMessageContent from blocks.
var NewUserMessageContentBlocks = message.NewUserMessageContentBlocks

// AssistantMessage represents a message from the model.
type AssistantMessage = message.AssistantMessage

// AssistantMessageError represents error types from the assistant.
type AssistantMessageError = message.AssistantMessageError

const (
	// AssistantMessageErrorAuthFailed indicates authentication failure.
	AssistantMessageErrorAuthFailed = message.AssistantMessageErrorAuthFailed
	// AssistantMessageErrorBilling indicates a billing error.
	AssistantMessageErrorBilling = message.AssistantMessageErrorBilling
	// AssistantMessageErrorRateLimit indicates rate limiting.
	AssistantMessageErrorRateLimit = message.AssistantMessageErrorRateLimit
	// AssistantMessageErrorInvalidReq indicates an invalid request.
	AssistantMessageErrorInvalidReq = message.AssistantMessageErrorInvalidReq
	// AssistantMessageErrorServer indicates a server error.
	AssistantMessageErrorServer = message.AssistantMessageErrorServer
	// AssistantMessageErrorUnknown indicates an unknown error.
	AssistantMessageErrorUnknown = message.AssistantMessageErrorUnknown
)

// SystemMessage represents a system message.
type SystemMessage = message.SystemMessage

// ResultMessage represents the final result of a query.
type ResultMessage = message.ResultMessage

// StreamEvent represents a raw streaming event from the LM Studio backend.
type StreamEvent = message.StreamEvent

// Usage contains token usage information.
type Usage = message.Usage

// ===== Content Blocks =====

const (
	// BlockTypeText contains plain text content.
	BlockTypeText = message.BlockTypeText
	// BlockTypeImage contains generated image output content.
	BlockTypeImage = message.BlockTypeImage
	// BlockTypeImageURL contains input image URL or data-url content.
	BlockTypeImageURL = message.BlockTypeImageURL
	// BlockTypeFile contains input file content.
	BlockTypeFile = message.BlockTypeFile
	// BlockTypeInputAudio contains input audio content.
	BlockTypeInputAudio = message.BlockTypeInputAudio
	// BlockTypeVideoURL contains input video URL or data-url content.
	BlockTypeVideoURL = message.BlockTypeVideoURL
	// BlockTypeThinking contains model reasoning content.
	BlockTypeThinking = message.BlockTypeThinking
	// BlockTypeToolUse contains tool-call content.
	BlockTypeToolUse = message.BlockTypeToolUse
	// BlockTypeToolResult contains tool-result content.
	BlockTypeToolResult = message.BlockTypeToolResult
)

// ContentBlock represents a block of content within a message.
type ContentBlock = message.ContentBlock

// TextBlock contains plain text content.
type TextBlock = message.TextBlock

// InputImageRef identifies an input image URL or data URL.
type InputImageRef = message.InputImageRef

// InputImageBlock contains an input image reference for multimodal prompts.
type InputImageBlock = message.InputImageBlock

// InputFileRef identifies an input file URL or data URL.
type InputFileRef = message.InputFileRef

// InputFileBlock contains an input file reference for multimodal prompts.
type InputFileBlock = message.InputFileBlock

// InputAudioRef contains base64-encoded input audio plus its format.
type InputAudioRef = message.InputAudioRef

// InputAudioBlock contains an input audio payload for multimodal prompts.
type InputAudioBlock = message.InputAudioBlock

// InputVideoRef identifies an input video URL or data URL.
type InputVideoRef = message.InputVideoRef

// InputVideoBlock contains an input video reference for multimodal prompts.
type InputVideoBlock = message.InputVideoBlock

// ImageBlock contains a generated image reference.
type ImageBlock = message.ImageBlock

// ThinkingBlock contains model reasoning or thinking output.
type ThinkingBlock = message.ThinkingBlock

// ToolUseBlock represents the model invoking a tool.
type ToolUseBlock = message.ToolUseBlock

// ToolResultBlock contains the result of a tool execution.
type ToolResultBlock = message.ToolResultBlock

// ===== Hooks =====

// HookEvent represents the type of event that triggers a hook.
type HookEvent = hook.Event

const (
	// HookEventPreToolUse is triggered before a tool is used.
	HookEventPreToolUse = hook.EventPreToolUse
	// HookEventPostToolUse is triggered after a tool is used.
	HookEventPostToolUse = hook.EventPostToolUse
	// HookEventUserPromptSubmit is triggered when a user submits a prompt.
	HookEventUserPromptSubmit = hook.EventUserPromptSubmit
	// HookEventStop is triggered when a session stops.
	HookEventStop = hook.EventStop
	// HookEventSubagentStop is triggered when a subagent stops.
	HookEventSubagentStop = hook.EventSubagentStop
	// HookEventPreCompact is triggered before compaction.
	HookEventPreCompact = hook.EventPreCompact
	// HookEventPostToolUseFailure is triggered after a tool use fails.
	HookEventPostToolUseFailure = hook.EventPostToolUseFailure
	// HookEventNotification is triggered when a notification is sent.
	HookEventNotification = hook.EventNotification
	// HookEventSubagentStart is triggered when a subagent starts.
	HookEventSubagentStart = hook.EventSubagentStart
	// HookEventPermissionRequest is triggered when a permission is requested.
	HookEventPermissionRequest = hook.EventPermissionRequest
)

// HookInput is the interface for all hook input types.
type HookInput = hook.Input

// BaseHookInput contains common fields for all hook inputs.
type BaseHookInput = hook.BaseInput

// PreToolUseHookInput is the input for PreToolUse hooks.
type PreToolUseHookInput = hook.PreToolUseInput

// PostToolUseHookInput is the input for PostToolUse hooks.
type PostToolUseHookInput = hook.PostToolUseInput

// UserPromptSubmitHookInput is the input for UserPromptSubmit hooks.
type UserPromptSubmitHookInput = hook.UserPromptSubmitInput

// StopHookInput is the input for Stop hooks.
type StopHookInput = hook.StopInput

// SubagentStopHookInput is the input for SubagentStop hooks.
type SubagentStopHookInput = hook.SubagentStopInput

// PreCompactHookInput is the input for PreCompact hooks.
type PreCompactHookInput = hook.PreCompactInput

// PostToolUseFailureHookInput is the input for PostToolUseFailure hooks.
type PostToolUseFailureHookInput = hook.PostToolUseFailureInput

// NotificationHookInput is the input for Notification hooks.
type NotificationHookInput = hook.NotificationInput

// SubagentStartHookInput is the input for SubagentStart hooks.
type SubagentStartHookInput = hook.SubagentStartInput

// PermissionRequestHookInput is the input for PermissionRequest hooks.
type PermissionRequestHookInput = hook.PermissionRequestInput

// HookJSONOutput is the interface for hook output types.
type HookJSONOutput = hook.JSONOutput

// AsyncHookJSONOutput represents an async hook output.
type AsyncHookJSONOutput = hook.AsyncJSONOutput

// SyncHookJSONOutput represents a sync hook output.
type SyncHookJSONOutput = hook.SyncJSONOutput

// HookSpecificOutput is the interface for hook-specific outputs.
type HookSpecificOutput = hook.SpecificOutput

// PreToolUseHookSpecificOutput is the hook-specific output for PreToolUse.
type PreToolUseHookSpecificOutput = hook.PreToolUseSpecificOutput

// PostToolUseHookSpecificOutput is the hook-specific output for PostToolUse.
type PostToolUseHookSpecificOutput = hook.PostToolUseSpecificOutput

// UserPromptSubmitHookSpecificOutput is the hook-specific output for UserPromptSubmit.
type UserPromptSubmitHookSpecificOutput = hook.UserPromptSubmitSpecificOutput

// PostToolUseFailureHookSpecificOutput is the hook-specific output for PostToolUseFailure.
type PostToolUseFailureHookSpecificOutput = hook.PostToolUseFailureSpecificOutput

// NotificationHookSpecificOutput is the hook-specific output for Notification.
type NotificationHookSpecificOutput = hook.NotificationSpecificOutput

// SubagentStartHookSpecificOutput is the hook-specific output for SubagentStart.
type SubagentStartHookSpecificOutput = hook.SubagentStartSpecificOutput

// PermissionRequestHookSpecificOutput is the hook-specific output for PermissionRequest.
type PermissionRequestHookSpecificOutput = hook.PermissionRequestSpecificOutput

// HookContext provides context for hook execution.
type HookContext = hook.Context

// HookCallback is the function signature for hook callbacks.
type HookCallback = hook.Callback

// HookMatcher configures which tools/events a hook applies to.
type HookMatcher = hook.Matcher

// ===== Permissions =====

// PermissionMode represents different permission handling modes.
type PermissionMode = permission.Mode

const (
	// PermissionModeDefault uses standard permission prompts.
	PermissionModeDefault = permission.ModeDefault
	// PermissionModeAcceptEdits automatically accepts file edits.
	PermissionModeAcceptEdits = permission.ModeAcceptEdits
	// PermissionModePlan enables plan mode for implementation planning.
	PermissionModePlan = permission.ModePlan
	// PermissionModeBypassPermissions bypasses all permission checks.
	PermissionModeBypassPermissions = permission.ModeBypassPermissions
)

// PermissionUpdateType represents the type of permission update.
type PermissionUpdateType = permission.UpdateType

const (
	// PermissionUpdateTypeAddRules adds new permission rules.
	PermissionUpdateTypeAddRules = permission.UpdateTypeAddRules
	// PermissionUpdateTypeReplaceRules replaces existing permission rules.
	PermissionUpdateTypeReplaceRules = permission.UpdateTypeReplaceRules
	// PermissionUpdateTypeRemoveRules removes permission rules.
	PermissionUpdateTypeRemoveRules = permission.UpdateTypeRemoveRules
	// PermissionUpdateTypeSetMode sets the permission mode.
	PermissionUpdateTypeSetMode = permission.UpdateTypeSetMode
	// PermissionUpdateTypeAddDirectories adds accessible directories.
	PermissionUpdateTypeAddDirectories = permission.UpdateTypeAddDirectories
	// PermissionUpdateTypeRemoveDirectories removes accessible directories.
	PermissionUpdateTypeRemoveDirectories = permission.UpdateTypeRemoveDirectories
)

// PermissionUpdateDestination represents where permission updates are stored.
type PermissionUpdateDestination = permission.UpdateDestination

const (
	// PermissionUpdateDestUserSettings stores in user-level settings.
	PermissionUpdateDestUserSettings = permission.UpdateDestUserSettings
	// PermissionUpdateDestProjectSettings stores in project-level settings.
	PermissionUpdateDestProjectSettings = permission.UpdateDestProjectSettings
	// PermissionUpdateDestLocalSettings stores in local-level settings.
	PermissionUpdateDestLocalSettings = permission.UpdateDestLocalSettings
	// PermissionUpdateDestSession stores in the current session only.
	PermissionUpdateDestSession = permission.UpdateDestSession
)

// PermissionBehavior represents the permission behavior for a rule.
type PermissionBehavior = permission.Behavior

const (
	// PermissionBehaviorAllow automatically allows the operation.
	PermissionBehaviorAllow = permission.BehaviorAllow
	// PermissionBehaviorDeny automatically denies the operation.
	PermissionBehaviorDeny = permission.BehaviorDeny
	// PermissionBehaviorAsk prompts the user for permission.
	PermissionBehaviorAsk = permission.BehaviorAsk
)

// PermissionRuleValue represents a permission rule.
type PermissionRuleValue = permission.RuleValue

// PermissionUpdate represents a permission update request.
type PermissionUpdate = permission.Update

// ToolPermissionContext provides context for tool permission callbacks.
type ToolPermissionContext = permission.Context

// PermissionResult is the interface for permission decision results.
type PermissionResult = permission.Result

// PermissionResultAllow represents an allow decision.
type PermissionResultAllow = permission.ResultAllow

// PermissionResultDeny represents a deny decision.
type PermissionResultDeny = permission.ResultDeny

// ToolPermissionCallback is called before each tool use for permission checking.
type ToolPermissionCallback = permission.Callback

// ===== User Input =====

// UserInputQuestionOption represents a selectable choice in a user-input question.
type UserInputQuestionOption = userinput.QuestionOption

// UserInputQuestion represents a single user-input prompt.
type UserInputQuestion = userinput.Question

// UserInputAnswer contains the user's response(s) to a single question.
type UserInputAnswer = userinput.Answer

// UserInputRequest contains parsed user-input payload data.
type UserInputRequest = userinput.Request

// UserInputResponse contains answers keyed by question ID.
type UserInputResponse = userinput.Response

// UserInputCallback handles user-input requests and returns answers.
type UserInputCallback = userinput.Callback

// ===== MCP Server Configuration =====

// MCPServerType represents the type of MCP server.
type MCPServerType = mcp.ServerType

const (
	// MCPServerTypeStdio uses stdio for communication.
	MCPServerTypeStdio = mcp.ServerTypeStdio
	// MCPServerTypeSSE uses Server-Sent Events.
	MCPServerTypeSSE = mcp.ServerTypeSSE
	// MCPServerTypeHTTP uses HTTP for communication.
	MCPServerTypeHTTP = mcp.ServerTypeHTTP
	// MCPServerTypeSDK uses the SDK interface.
	MCPServerTypeSDK = mcp.ServerTypeSDK
)

// MCPServerConfig is the interface for MCP server configurations.
type MCPServerConfig = mcp.ServerConfig

// MCPStdioServerConfig configures a stdio-based MCP server.
type MCPStdioServerConfig = mcp.StdioServerConfig

// MCPSSEServerConfig configures a Server-Sent Events MCP server.
type MCPSSEServerConfig = mcp.SSEServerConfig

// MCPHTTPServerConfig configures an HTTP-based MCP server.
type MCPHTTPServerConfig = mcp.HTTPServerConfig

// MCPSdkServerConfig configures an SDK-provided MCP server.
type MCPSdkServerConfig = mcp.SdkServerConfig

// SdkMcpServerInstance is the interface that SDK MCP servers must implement.
type SdkMcpServerInstance = mcp.ServerInstance

// ===== Model Discovery =====

// Model is a stable provider-neutral projection of a discovered model.
type Model = model.Model

// ModelInfo describes a model available from LM Studio discovery endpoints.
type ModelInfo = model.Info

// ModelArchitecture describes the underlying model family where available.
type ModelArchitecture = model.Architecture

// ModelPricing contains pricing fields as returned by discovery endpoints.
type ModelPricing = model.Pricing

// ModelTopProvider describes provider-side limits and moderation behavior.
type ModelTopProvider = model.TopProvider

// ModelPerRequestLimits captures request limits where available.
type ModelPerRequestLimits = model.PerRequestLimits

// ModelSupportedParameters records provider-supported request parameters.
type ModelSupportedParameters = model.SupportedParameters

// ModelEndpoint identifies an endpoint a model supports.
type ModelEndpoint = model.Endpoint

// ModelListResponse contains the full model discovery payload.
type ModelListResponse = model.ListResponse

// ===== MCP Status =====

// MCPServerStatus represents the connection status of a single MCP server.
type MCPServerStatus = mcp.ServerStatus

// MCPStatus represents the connection status of all configured MCP servers.
type MCPStatus = mcp.Status

// ===== Streaming Input =====

// MessageStream is an iterator that yields streaming messages.
type MessageStream = iter.Seq[StreamingMessage]

// StreamingMessage represents a message sent in streaming mode.
type StreamingMessage = message.StreamingMessage

// StreamingMessageContent represents the content of a streaming message.
type StreamingMessageContent = message.StreamingMessageContent
