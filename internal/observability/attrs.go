package observability

import (
	"time"

	"go.opentelemetry.io/otel/attribute"
)

// SDK-local attribute keys. Not covered by any OTel spec; OpenRouter-specific
// operational labels. Value sets must stay bounded (cardinality discipline).
var (
	StatusClassKey    = attribute.Key("status_class")
	RetryKey          = attribute.Key("retry")
	RetryAttemptKey   = attribute.Key("retry.attempt")
	RetryDelayKey     = attribute.Key("retry.delay")
	OutcomeKey        = attribute.Key("outcome")
	CheckpointOpKey   = attribute.Key("checkpoint.op")
	HookEventKey      = attribute.Key("hook.event")
	ThinkingTokensKey = attribute.Key("thinking.tokens")
)

func StatusClass(v string) attribute.KeyValue { return StatusClassKey.String(v) }
func Retry(v bool) attribute.KeyValue         { return RetryKey.Bool(v) }
func RetryAttempt(v int) attribute.KeyValue   { return RetryAttemptKey.Int(v) }
func RetryDelay(v time.Duration) attribute.KeyValue {
	return RetryDelayKey.String(v.String())
}
func Outcome(v string) attribute.KeyValue       { return OutcomeKey.String(v) }
func CheckpointOp(v string) attribute.KeyValue  { return CheckpointOpKey.String(v) }
func HookEvent(v string) attribute.KeyValue     { return HookEventKey.String(v) }
func ThinkingTokens(v int64) attribute.KeyValue { return ThinkingTokensKey.Int64(v) }

// FinishReasons returns the gen_ai.response.finish_reasons span attribute.
// Per GenAI semconv this is a string array; OpenRouter has a single stop reason.
func FinishReasons(reasons ...string) attribute.KeyValue {
	return attribute.StringSlice("gen_ai.response.finish_reasons", reasons)
}
