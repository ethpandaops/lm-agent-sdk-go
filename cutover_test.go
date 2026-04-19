package lmsdk

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type scriptedTransport struct {
	t       *testing.T
	mu      sync.Mutex
	call    int
	scripts []func(*ChatRequest) ([]map[string]any, error)
}

func (t *scriptedTransport) Start(context.Context) error { return nil }
func (t *scriptedTransport) Close() error                { return nil }

func (t *scriptedTransport) CreateStream(
	_ context.Context,
	req *ChatRequest,
) (<-chan map[string]any, <-chan error) {
	t.mu.Lock()
	index := t.call
	t.call++
	t.mu.Unlock()

	out := make(chan map[string]any, 8)
	errs := make(chan error, 1)

	go func() {
		defer close(out)
		defer close(errs)

		if index >= len(t.scripts) {
			t.t.Errorf("unexpected transport call %d", index+1)
			return
		}
		events, err := t.scripts[index](req)
		if err != nil {
			errs <- err
			return
		}
		for _, event := range events {
			out <- event
		}
	}()

	return out, errs
}

func TestQuery_WithOnUserInputHandlesPromptTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var callbackReq *UserInputRequest
	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(req *ChatRequest) ([]map[string]any, error) {
				if len(req.Tools) == 0 {
					t.Fatalf("expected auto-registered user input tool")
				}
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
												"name":      "mcp__sdk__stdio",
												"arguments": `{"questions":[{"id":"approval","header":"Approval","question":"Continue?","options":[{"label":"yes","description":"continue"}]}]}`,
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
			func(req *ChatRequest) ([]map[string]any, error) {
				foundToolResult := false
				for _, msg := range req.Messages {
					if msg["role"] != "tool" {
						continue
					}
					content, _ := msg["content"].(string)
					if strings.Contains(content, `"approval":["yes"]`) {
						foundToolResult = true
					}
				}
				if !foundToolResult {
					t.Fatalf("expected serialized user input tool result in request history: %#v", req.Messages)
				}
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "continued"},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	var gotResult *ResultMessage
	for msg, err := range Query(ctx, Text("Ask for confirmation before continuing."),
		WithTransport(tr),
		WithOnUserInput(func(_ context.Context, req *UserInputRequest) (*UserInputResponse, error) {
			callbackReq = req
			return &UserInputResponse{
				Answers: map[string]*UserInputAnswer{
					"approval": {Answers: []string{"yes"}},
				},
			}, nil
		}),
		WithMaxTurns(3),
	) {
		if err != nil {
			t.Fatalf("unexpected query error: %v", err)
		}
		if result, ok := msg.(*ResultMessage); ok {
			gotResult = result
		}
	}

	if callbackReq == nil || len(callbackReq.Questions) != 1 {
		t.Fatalf("expected callback request, got %#v", callbackReq)
	}
	if callbackReq.Questions[0].ID != "approval" {
		t.Fatalf("unexpected callback question: %#v", callbackReq.Questions[0])
	}
	if gotResult == nil || gotResult.Result == nil || *gotResult.Result != "continued" {
		t.Fatalf("unexpected result: %#v", gotResult)
	}
}

func TestQuery_OnUserInputConflictYieldsError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, err := range Query(ctx, Text("hi"),
		WithOnUserInput(func(context.Context, *UserInputRequest) (*UserInputResponse, error) {
			return nil, nil
		}),
		WithPermissionPromptToolName("custom"),
	) {
		if err == nil {
			continue
		}
		if !strings.Contains(err.Error(), "on_user_input callback cannot be used with permission_prompt_tool_name") {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}

	t.Fatal("expected iterator error")
}

func TestQuery_WithSDKToolsExecutesEndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(req *ChatRequest) ([]map[string]any, error) {
				foundTool := false
				for _, tool := range req.Tools {
					function, _ := tool["function"].(map[string]any)
					if function["name"] == "mcp__sdk__echo" {
						foundTool = true
					}
				}
				if !foundTool {
					t.Fatalf("expected registered sdk tool in request: %#v", req.Tools)
				}
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
			func(req *ChatRequest) ([]map[string]any, error) {
				foundToolResult := false
				for _, msg := range req.Messages {
					if msg["role"] != "tool" {
						continue
					}
					content, _ := msg["content"].(string)
					if strings.Contains(content, `"echo":"hello"`) {
						foundToolResult = true
					}
				}
				if !foundToolResult {
					t.Fatalf("expected sdk tool result in request history: %#v", req.Messages)
				}
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "tool complete"},
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

	var gotResult *ResultMessage
	for msg, err := range Query(ctx, Text("Use the echo tool."), WithTransport(tr), WithSDKTools(tool), WithMaxTurns(3)) {
		if err != nil {
			t.Fatalf("unexpected query error: %v", err)
		}
		if result, ok := msg.(*ResultMessage); ok {
			gotResult = result
		}
	}

	if gotResult == nil || gotResult.Result == nil || *gotResult.Result != "tool complete" {
		t.Fatalf("unexpected result: %#v", gotResult)
	}
}

func TestSessionMetadataAPIsUseLocalStore(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	store := t.TempDir() + "/sessions.json"
	tr := &scriptedTransport{
		t: t,
		scripts: []func(*ChatRequest) ([]map[string]any, error){
			func(*ChatRequest) ([]map[string]any, error) {
				return []map[string]any{
					{
						"choices": []any{
							map[string]any{
								"delta":         map[string]any{"content": "stored"},
								"finish_reason": "stop",
							},
						},
					},
				}, nil
			},
		},
	}

	for _, err := range Query(ctx, Text("persist this session"), WithTransport(tr), WithSessionStorePath(store), WithMaxTurns(1)) {
		if err != nil {
			t.Fatalf("unexpected query error: %v", err)
		}
	}

	stat, err := StatSession(ctx, "default", WithSessionStorePath(store))
	if err != nil {
		t.Fatalf("stat session: %v", err)
	}
	if stat.SessionID != "default" || stat.MessageCount != 2 || stat.UserTurns != 1 {
		t.Fatalf("unexpected session stat: %+v", stat)
	}

	sessions, err := ListSessions(ctx, WithSessionStorePath(store))
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	if len(sessions) != 1 || sessions[0].SessionID != "default" {
		t.Fatalf("unexpected sessions: %+v", sessions)
	}

	messages, err := GetSessionMessages(ctx, "default", WithSessionStorePath(store))
	if err != nil {
		t.Fatalf("get session messages: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("expected 2 persisted messages, got %d", len(messages))
	}
	if _, ok := messages[0].(*UserMessage); !ok {
		t.Fatalf("expected first message to be user, got %T", messages[0])
	}
	if _, ok := messages[1].(*AssistantMessage); !ok {
		t.Fatalf("expected second message to be assistant, got %T", messages[1])
	}
}

func TestListModelsResponseUsesNativeEndpointAndFallback(t *testing.T) {
	var mu sync.Mutex
	var paths []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		paths = append(paths, r.URL.Path)
		mu.Unlock()

		switch r.URL.Path {
		case "/api/v0/models":
			if r.Header.Get("Authorization") == "" {
				http.Error(w, "missing auth", http.StatusUnauthorized)
				return
			}
			writeModelResponse(t, w, []map[string]any{
				{
					"id":                 "qwen/qwen3.6-35b-a3b",
					"type":               "vlm",
					"publisher":          "qwen",
					"arch":               "qwen35moe",
					"compatibility_type": "gguf",
					"quantization":       "Q4_K_M",
					"state":              "loaded",
					"max_context_length": 26565,
					"capabilities":       []string{"tool_use"},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	resp, err := ListModelsResponse(ctx, WithBaseURL(server.URL+"/v1"), WithAPIKey("test-key"))
	if err != nil {
		t.Fatalf("list models response: %v", err)
	}
	if !resp.Authenticated || len(resp.Models) != 1 {
		t.Fatalf("unexpected authenticated response: %+v", resp)
	}
	if !resp.Models[0].SupportsToolCalling() || resp.Models[0].CostTier() != "free" {
		t.Fatalf("unexpected model helpers: %+v", resp.Models[0])
	}
	if got := resp.Models[0].MaxContextLength(); got != 26565 {
		t.Fatalf("unexpected max context length: %d", got)
	}
	if resp.Models[0].Quantization != "Q4_K_M" {
		t.Fatalf("expected Quantization Q4_K_M, got %q", resp.Models[0].Quantization)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(paths) != 1 || paths[0] != "/api/v0/models" {
		t.Fatalf("unexpected requested paths: %#v", paths)
	}
}

func TestClientUnsupportedControlsReturnTypedErrors(t *testing.T) {
	client := NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Start(ctx, WithTransport(&scriptedTransport{t: t}), WithAPIKey("unused")); err != nil {
		t.Fatalf("start client: %v", err)
	}
	defer func() { _ = client.Close() }()

	checks := []error{
		client.ReconnectMCPServer(ctx, "srv"),
		client.ToggleMCPServer(ctx, "srv", true),
		client.StopTask(ctx, "task-1"),
		client.SendToolResult(ctx, "tool-1", "{}", false),
	}
	for _, err := range checks {
		var unsupported *UnsupportedControlError
		if err == nil || !strings.Contains(err.Error(), "unsupported") && !strings.Contains(err.Error(), "not") {
			t.Fatalf("expected unsupported control error, got %v", err)
		}
		if !errors.As(err, &unsupported) {
			t.Fatalf("expected typed unsupported control error, got %T", err)
		}
	}
}

func writeModelResponse(t *testing.T, w http.ResponseWriter, data []map[string]any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"data": data}); err != nil {
		t.Fatalf("encode model response: %v", err)
	}
}
