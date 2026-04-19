package tools

import "testing"

func TestRegistry_ResolveName(t *testing.T) {
	r := &Registry{
		byName: map[string]ToolRef{
			"mcp__sdk__add":       {Server: "sdk", Tool: "add"},
			"mcp__calc__multiply": {Server: "calc", Tool: "multiply"},
			"short_name":          {Server: "x", Tool: "short_name"},
		},
	}

	tests := []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"mcp__sdk__add", "mcp__sdk__add", true},             // direct hit
		{"mcp_sdk_add", "mcp__sdk__add", true},               // collapsed match (Qwen3 XML style)
		{"mcp__calc__multiply", "mcp__calc__multiply", true}, // direct hit
		{"mcp_calc_multiply", "mcp__calc__multiply", true},   // collapsed match
		{"short_name", "short_name", true},                   // direct hit, no collapse needed
		{"unknown_tool", "", false},                          // no match
		{"", "", false},                                      // empty
	}
	for _, tc := range tests {
		got, ok := r.ResolveName(tc.in)
		if ok != tc.wantOK || got != tc.want {
			t.Errorf("ResolveName(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestCollapseUnderscores(t *testing.T) {
	tests := map[string]string{
		"":                       "",
		"no_underscores":         "no_underscores",
		"foo__bar":               "foo_bar",
		"foo___bar":              "foo_bar",
		"foo__bar__baz":          "foo_bar_baz",
		"mcp__server__tool_name": "mcp_server_tool_name",
		"____":                   "_",
	}
	for in, want := range tests {
		if got := collapseUnderscores(in); got != want {
			t.Errorf("collapseUnderscores(%q) = %q, want %q", in, got, want)
		}
	}
}
