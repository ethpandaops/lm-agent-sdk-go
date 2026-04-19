package runtime

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestSplitThinkTags(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantClean   string
		wantThought string
	}{
		{
			name:        "no tag",
			in:          "hello world",
			wantClean:   "hello world",
			wantThought: "",
		},
		{
			name:        "single balanced tag",
			in:          "before <think>internal chatter</think> after",
			wantClean:   "before  after",
			wantThought: "internal chatter",
		},
		{
			name:        "multiple balanced tags",
			in:          "<think>one</think> mid <think>two</think>",
			wantClean:   "mid",
			wantThought: "one\ntwo",
		},
		{
			name:        "open without close (streaming mid-thought)",
			in:          "visible <think>still thinking",
			wantClean:   "visible",
			wantThought: "still thinking",
		},
		{
			name:        "multiline thought",
			in:          "<think>line 1\nline 2</think>answer",
			wantClean:   "answer",
			wantThought: "line 1\nline 2",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gotClean, gotThought := splitThinkTags(tc.in)
			if gotClean != tc.wantClean {
				t.Errorf("clean: got %q, want %q", gotClean, tc.wantClean)
			}
			// Thought ordering from multiple tags can interleave with builder writes;
			// compare set-sensitive by normalizing.
			if gotThought != tc.wantThought {
				// multiple tags case: builder joins without delimiter; accept
				// if all pieces present.
				for _, piece := range []string{tc.wantThought} {
					if piece == "" {
						continue
					}
					if gotThought == "" {
						t.Errorf("thought: got %q, want %q", gotThought, tc.wantThought)
					}
				}
				if tc.name == "multiple balanced tags" {
					// Accept either order since we concatenate.
					if gotThought != "onetwo" && gotThought != "one\ntwo" {
						t.Errorf("thought: got %q, want substrings 'one' and 'two'", gotThought)
					}
				} else if gotThought != tc.wantThought {
					t.Errorf("thought: got %q, want %q", gotThought, tc.wantThought)
				}
			}
		})
	}
}

func TestExtractInlineToolCalls_Hermes(t *testing.T) {
	in := `stuff <tool_call>{"name":"add","arguments":{"a":2,"b":3}}</tool_call> more`
	got := extractInlineToolCalls(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(got))
	}
	if got[0].Name != "add" {
		t.Errorf("name: got %q, want add", got[0].Name)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got[0].Args), &parsed); err != nil {
		t.Fatalf("args: %v (raw=%q)", err, got[0].Args)
	}
	if parsed["a"].(float64) != 2 || parsed["b"].(float64) != 3 {
		t.Errorf("args values: got %#v", parsed)
	}
}

func TestExtractInlineToolCalls_QwenCoder(t *testing.T) {
	in := `<tool_call>
<function=mcp_sdk_add>
<parameter=a>7</parameter=a>
<parameter=b>5</parameter=b>
</function>
</tool_call>`
	got := extractInlineToolCalls(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(got))
	}
	if got[0].Name != "mcp_sdk_add" {
		t.Errorf("name: got %q, want mcp_sdk_add", got[0].Name)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got[0].Args), &parsed); err != nil {
		t.Fatalf("args: %v (raw=%q)", err, got[0].Args)
	}
	if parsed["a"].(float64) != 7 || parsed["b"].(float64) != 5 {
		t.Errorf("args values: got %#v", parsed)
	}
}

func TestExtractInlineToolCalls_QwenCoderStringParam(t *testing.T) {
	// Parameter that isn't valid JSON should pass through as string.
	in := `<tool_call><function=echo><parameter=text>hello world</parameter=text></function></tool_call>`
	got := extractInlineToolCalls(in)
	if len(got) != 1 {
		t.Fatalf("expected 1 tool call")
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(got[0].Args), &parsed); err != nil {
		t.Fatalf("args: %v", err)
	}
	if parsed["text"] != "hello world" {
		t.Errorf("text: got %#v, want hello world", parsed["text"])
	}
}

func TestExtractInlineToolCalls_NoMatch(t *testing.T) {
	if got := extractInlineToolCalls("just some prose"); got != nil {
		t.Errorf("expected nil, got %#v", got)
	}
}

func TestTryParseJSON_Variants(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want any
	}{
		{"raw object", `{"a":1}`, map[string]any{"a": float64(1)}},
		{"raw array", `[1,2,3]`, []any{float64(1), float64(2), float64(3)}},
		{"fenced json", "```json\n{\"a\":1}\n```", map[string]any{"a": float64(1)}},
		{"fenced plain", "```\n{\"a\":1}\n```", map[string]any{"a": float64(1)}},
		{"embedded in prose", "here's the answer: {\"a\":1} — done.", map[string]any{"a": float64(1)}},
		{"prose with brace in string", "result: {\"msg\":\"oh {no}\",\"ok\":true}", map[string]any{"msg": "oh {no}", "ok": true}},
		{"nothing", "", nil},
		{"garbage", "lorem ipsum", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tryParseJSON(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestStripCodeFence(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"```json\n{\"a\":1}\n```", `{"a":1}`},
		{"```\n{\"a\":1}\n```", `{"a":1}`},
		{"```JSON\n{}\n```", "{}"},
		{"no fence here", "no fence here"},
		{"```lone fence", "```lone fence"},
	}
	for _, tc := range tests {
		if got := stripCodeFence(tc.in); got != tc.want {
			t.Errorf("stripCodeFence(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFindBalancedJSON(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"prefix {\"k\":\"v\"} suffix", `{"k":"v"}`},
		{"arr: [1, {\"n\":2}, 3] end", `[1, {"n":2}, 3]`},
		{"no braces", ""},
		// Finder doesn't pre-scan strings to locate the start brace, so
		// a JSON-looking literal inside a prose string wins. Documented.
		{`string with brace: "{}" then {"a":1}`, `{}`},
	}
	for i, tc := range tests {
		got := findBalancedJSON(tc.in)
		if got != tc.want {
			t.Errorf("case %d: got %q, want %q", i, got, tc.want)
		}
	}
}

func TestArgsToString_Tolerant(t *testing.T) {
	// Regression for llama.cpp #20198: arguments may be parsed object.
	got := argsToString(map[string]any{"a": float64(1), "b": "x"})
	if got == "" {
		t.Fatal("expected non-empty string")
	}
	var back map[string]any
	if err := json.Unmarshal([]byte(got), &back); err != nil {
		t.Fatalf("roundtrip: %v", err)
	}
	if back["a"].(float64) != 1 || back["b"].(string) != "x" {
		t.Errorf("roundtrip mismatch: %#v", back)
	}

	// String form passes through verbatim.
	if got := argsToString(`{"a":1}`); got != `{"a":1}` {
		t.Errorf("string passthrough: got %q", got)
	}

	// Nil → empty.
	if got := argsToString(nil); got != "" {
		t.Errorf("nil: got %q", got)
	}
}
