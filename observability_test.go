package lmsdk

import (
	"context"
	"testing"
	"time"

	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

// newTestProviders creates in-memory OTel providers for test assertions.
func newTestProviders() (
	*sdkmetric.MeterProvider,
	*sdkmetric.ManualReader,
	*sdktrace.TracerProvider,
	*tracetest.InMemoryExporter,
) {
	reader := sdkmetric.NewManualReader()
	mp := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	spanExporter := tracetest.NewInMemoryExporter()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSyncer(spanExporter))

	return mp, reader, tp, spanExporter
}

// collectMetrics reads all metrics from the manual reader into a ResourceMetrics.
func collectMetrics(t *testing.T, reader *sdkmetric.ManualReader) metricdata.ResourceMetrics {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(context.Background(), &rm); err != nil {
		t.Fatalf("collect metrics: %v", err)
	}
	return rm
}

// findMetric searches for a metric by name in ResourceMetrics.
func findMetric(rm metricdata.ResourceMetrics, name string) *metricdata.Metrics {
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			if m.Name == name {
				return &m
			}
		}
	}
	return nil
}

// findSpanPrefix searches for a span whose name starts with prefix.
func findSpanPrefix(spans tracetest.SpanStubs, prefix string) *tracetest.SpanStub {
	for _, s := range spans {
		if len(s.Name) >= len(prefix) && s.Name[:len(prefix)] == prefix {
			return &s
		}
	}
	return nil
}

func TestObservability_QueryMetrics(t *testing.T) {
	mp, reader, tp, spanExporter := newTestProviders()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "hello"},
								"finish_reason": "stop",
							},
						},
						"usage": map[string]any{
							"prompt_tokens":     float64(10),
							"completion_tokens": float64(5),
							"total_tokens":      float64(15),
						},
					},
				}, nil
			},
		},
	}

	for _, err := range Query(ctx, Text("hi"),
		WithTransport(tr),
		WithMeterProvider(mp),
		WithTracerProvider(tp),
		WithMaxTurns(1),
	) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
	}

	// Check metrics.
	rm := collectMetrics(t, reader)

	if m := findMetric(rm, "gen_ai.client.operation.duration"); m == nil {
		t.Error("expected gen_ai.client.operation.duration metric")
	}

	if m := findMetric(rm, "gen_ai.client.token.usage"); m == nil {
		t.Error("expected gen_ai.client.token.usage metric")
	}

	// Check spans — GenAI semconv "chat {model}" or just "chat".
	if err := tp.ForceFlush(ctx); err != nil {
		t.Fatalf("force flush: %v", err)
	}
	spans := spanExporter.GetSpans()
	if qs := findSpanPrefix(spans, "chat"); qs == nil {
		t.Error("expected chat query span")
	}
}

func TestObservability_ToolCallMetrics(t *testing.T) {
	mp, reader, tp, spanExporter := newTestProviders()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta": map[string]any{
									"tool_calls": []any{
										map[string]any{
											"index": 0.0,
											"id":    "call_1",
											"function": map[string]any{
												"name":      "mcp__sdk__echo",
												"arguments": `{"text":"hello"}`,
											},
										},
									},
								},
								"finish_reason": "tool_calls",
							},
						},
					},
				}, nil
			},
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "done"},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	tool := NewTool("echo", "Echo text", map[string]any{
		"type": "object",
		"properties": map[string]any{
			"text": map[string]any{"type": "string"},
		},
		"required": []string{"text"},
	}, func(_ context.Context, input map[string]any) (map[string]any, error) {
		return map[string]any{"echo": input["text"]}, nil
	})

	for _, err := range Query(ctx, Text("echo something"),
		WithTransport(tr),
		WithMeterProvider(mp),
		WithTracerProvider(tp),
		WithSDKTools(tool),
		WithMaxTurns(3),
	) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
	}

	// Check tool call metrics.
	rm := collectMetrics(t, reader)

	if m := findMetric(rm, "lmstudio.tool_calls_total"); m == nil {
		t.Error("expected lmstudio.tool_calls_total metric")
	} else {
		sum, ok := m.Data.(metricdata.Sum[int64])
		if !ok {
			t.Fatalf("expected Sum[int64] for tool_calls_total, got %T", m.Data)
		}
		if len(sum.DataPoints) == 0 {
			t.Fatal("expected tool_calls_total data points")
		}
		var found bool
		for _, dp := range sum.DataPoints {
			for _, attr := range dp.Attributes.ToSlice() {
				if attr.Key == "outcome" && attr.Value.AsString() == "ok" {
					found = true
				}
			}
		}
		if !found {
			t.Error("expected tool_calls_total with outcome=ok")
		}
	}

	if m := findMetric(rm, "lmstudio.tool_call_duration"); m == nil {
		t.Error("expected lmstudio.tool_call_duration metric")
	}

	// Check spans — GenAI semconv "execute_tool {name}".
	if err := tp.ForceFlush(ctx); err != nil {
		t.Fatalf("force flush: %v", err)
	}
	spans := spanExporter.GetSpans()
	if ts := findSpanPrefix(spans, "execute_tool"); ts == nil {
		t.Error("expected execute_tool span")
	} else {
		var foundToolName bool
		for _, attr := range ts.Attributes {
			if attr.Key == "gen_ai.tool.name" && attr.Value.AsString() == "mcp__sdk__echo" {
				foundToolName = true
			}
		}
		if !foundToolName {
			t.Error("expected execute_tool span to have gen_ai.tool.name attribute")
		}
	}
}

func TestObservability_StreamingTTFT(t *testing.T) {
	mp, reader, _, _ := newTestProviders()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta": map[string]any{"content": "first "},
							},
						},
					},
					{
						"choices": []any{
							map[string]any{
								"delta": map[string]any{"content": "second "},
							},
						},
					},
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "third"},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	for _, err := range Query(ctx, Text("hi"),
		WithTransport(tr),
		WithMeterProvider(mp),
		WithMaxTurns(1),
	) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
	}

	rm := collectMetrics(t, reader)

	if m := findMetric(rm, "gen_ai.client.operation.time_to_first_chunk"); m == nil {
		t.Error("expected gen_ai.client.operation.time_to_first_chunk metric")
	} else {
		hist, ok := m.Data.(metricdata.Histogram[float64])
		if !ok {
			t.Fatalf("expected Histogram[float64] for ttft, got %T", m.Data)
		}
		if len(hist.DataPoints) == 0 {
			t.Fatal("expected ttft data points")
		}
		if hist.DataPoints[0].Count != 1 {
			t.Errorf("expected exactly 1 ttft recording, got %d", hist.DataPoints[0].Count)
		}
	}
}

func TestObservability_SpanHierarchy(t *testing.T) {
	_, _, tp, spanExporter := newTestProviders()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "done"},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	for _, err := range Query(ctx, Text("hi"),
		WithTransport(tr),
		WithTracerProvider(tp),
		WithMaxTurns(1),
	) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
	}

	if err := tp.ForceFlush(ctx); err != nil {
		t.Fatalf("force flush: %v", err)
	}
	spans := spanExporter.GetSpans()

	querySpan := findSpanPrefix(spans, "chat")
	if querySpan == nil {
		t.Fatal("expected chat query span")
	}
}

func TestObservability_NoopSafety(t *testing.T) {
	// Ensure no panic when providers are nil (default noop).
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "ok"},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	for _, err := range Query(ctx, Text("noop test"),
		WithTransport(tr),
		WithMaxTurns(1),
	) {
		if err != nil {
			t.Fatalf("query error with nil providers: %v", err)
		}
	}
}

func TestObservability_PrometheusRegisterer(t *testing.T) {
	// Integration test for WithPrometheusRegisterer.
	promclient := newTestPrometheusRegistry()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "hello"},
								"finish_reason": "stop",
							},
						},
						"usage": map[string]any{
							"prompt_tokens":     float64(10),
							"completion_tokens": float64(5),
							"total_tokens":      float64(15),
						},
					},
				}, nil
			},
		},
	}

	for _, err := range Query(ctx, Text("prom test"),
		WithTransport(tr),
		WithPrometheusRegisterer(promclient),
		WithMaxTurns(1),
	) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
	}

	// Verify that metrics were registered with the prometheus registry by
	// gathering all metrics and checking that at least one OTel metric
	// is present.
	families, err := promclient.Gather()
	if err != nil {
		t.Fatalf("prometheus gather: %v", err)
	}

	found := false
	for _, f := range families {
		name := f.GetName()
		// Prometheus adds unit suffixes (_seconds, _total) to metric names.
		if name == "gen_ai_client_operation_duration_seconds" ||
			name == "gen_ai_client_token_usage" {
			found = true
			break
		}
	}
	if !found {
		names := make([]string, 0, len(families))
		for _, f := range families {
			names = append(names, f.GetName())
		}
		t.Errorf("expected OTel metric in prometheus registry, found families: %v", names)
	}
}

func TestObservability_HookMetrics(t *testing.T) {
	mp, reader, _, _ := newTestProviders()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "ok"},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	hookCalled := false
	hooks := map[HookEvent][]*HookMatcher{
		HookEventStop: {
			{
				Hooks: []HookCallback{
					func(_ context.Context, _ HookInput, _ *string, _ *HookContext) (HookJSONOutput, error) {
						hookCalled = true
						return nil, nil
					},
				},
			},
		},
	}

	for _, err := range Query(ctx, Text("hook test"),
		WithTransport(tr),
		WithMeterProvider(mp),
		WithHooks(hooks),
		WithMaxTurns(1),
	) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
	}

	if !hookCalled {
		t.Fatal("expected hook to be called")
	}

	rm := collectMetrics(t, reader)
	if m := findMetric(rm, "lmstudio.hook_dispatch_duration"); m == nil {
		t.Error("expected lmstudio.hook_dispatch_duration metric")
	} else {
		hist, ok := m.Data.(metricdata.Histogram[float64])
		if !ok {
			t.Fatalf("expected Histogram[float64] for hook_dispatch_duration, got %T", m.Data)
		}
		if len(hist.DataPoints) == 0 {
			t.Fatal("expected hook_dispatch_duration data points")
		}
		// Check that stop event is recorded.
		var foundStop bool
		for _, dp := range hist.DataPoints {
			for _, attr := range dp.Attributes.ToSlice() {
				if attr.Key == "hook.event" && attr.Value.AsString() == string(HookEventStop) {
					foundStop = true
				}
			}
		}
		if !foundStop {
			t.Error("expected hook_dispatch_duration with hook.event=stop")
		}
	}
}

func TestObservability_ErrorClassification(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected string
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: "",
		},
		{
			name:     "request timeout",
			err:      ErrRequestTimeout,
			expected: "timeout",
		},
		{
			name: "tool permission denied",
			err: &ToolPermissionDeniedError{
				ToolName: "test", Message: "denied",
			},
			expected: "permission_denied",
		},
		{
			name: "unsupported hook event",
			err: &UnsupportedHookEventError{
				Event: "foo",
			},
			expected: "unsupported_hook_event",
		},
		{
			name: "unsupported hook output",
			err: &UnsupportedHookOutputError{
				Event: "foo", Field: "bar",
			},
			expected: "unsupported_hook_output",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := classifyErrorForTest(tc.err)
			if result != tc.expected {
				t.Errorf("Classify(%v) = %q, want %q", tc.err, result, tc.expected)
			}
		})
	}
}

func TestObservability_StatusClass(t *testing.T) {
	tests := []struct {
		code     int
		expected string
	}{
		{200, "2xx"},
		{201, "2xx"},
		{301, "3xx"},
		{400, "4xx"},
		{429, "4xx"},
		{500, "5xx"},
		{503, "5xx"},
		{100, "other"},
	}

	for _, tc := range tests {
		result := statusClassForTest(tc.code)
		if result != tc.expected {
			t.Errorf("StatusClassOf(%d) = %q, want %q", tc.code, result, tc.expected)
		}
	}
}

func TestObservability_ModelWithAttributes(t *testing.T) {
	_, _, tp, spanExporter := newTestProviders()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "ok"},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	for _, err := range Query(ctx, Text("attr test"),
		WithTransport(tr),
		WithTracerProvider(tp),
		WithModel("test-model"),
		WithMaxTurns(1),
	) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
	}

	if err := tp.ForceFlush(ctx); err != nil {
		t.Fatalf("force flush: %v", err)
	}
	spans := spanExporter.GetSpans()

	querySpan := findSpanPrefix(spans, "chat")
	if querySpan == nil {
		t.Fatal("expected chat query span")
	}

	var modelAttr attribute.KeyValue
	for _, a := range querySpan.Attributes {
		if a.Key == "gen_ai.request.model" {
			modelAttr = a
		}
	}
	if modelAttr.Key == "" {
		t.Error("expected gen_ai.request.model attribute on query span")
	} else if modelAttr.Value.AsString() != "test-model" {
		t.Errorf("expected model=test-model, got %s", modelAttr.Value.AsString())
	}
}
