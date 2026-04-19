package tools

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
)

// ToolRef maps a public tool name to backing MCP server/tool.
type ToolRef struct {
	Server string
	Tool   string
	Desc   string
	Schema map[string]any
	Kind   string // "sdk" or "external"
}

// Registry contains tool metadata for model exposure and execution.
type Registry struct {
	byName       map[string]ToolRef
	serverStatus map[string]mcp.ServerStatus
}

// NewRegistry builds a registry from options.
func NewRegistry(opts *config.Options) *Registry {
	r := &Registry{
		byName:       make(map[string]ToolRef),
		serverStatus: make(map[string]mcp.ServerStatus),
	}
	if opts == nil || len(opts.MCPServers) == 0 {
		return r
	}

	allowedSet := toSet(opts.AllowedTools)
	disallowedSet := toSet(opts.DisallowedTools)
	configuredToolsSet, enforceConfiguredTools := toConfiguredToolsSet(opts.Tools)
	enforceAllow := len(allowedSet) > 0

	for serverName, cfg := range opts.MCPServers {
		status := mcp.ServerStatus{Name: serverName, Status: "configured"}
		if cfg == nil {
			status.Status = "error"
			status.Reason = "missing config"
			r.serverStatus[serverName] = status
			continue
		}

		switch sc := cfg.(type) {
		case *mcp.SdkServerConfig:
			if sc == nil || sc.Instance == nil {
				status.Status = "error"
				status.Reason = "missing sdk server instance"
				r.serverStatus[serverName] = status
				continue
			}
			inst, ok := sc.Instance.(mcp.ServerInstance)
			if !ok {
				status.Status = "error"
				status.Reason = "invalid sdk server instance type"
				r.serverStatus[serverName] = status
				continue
			}
			status.Status = "ready"
			r.serverStatus[serverName] = status
			for _, t := range inst.ListTools() {
				name, _ := t["name"].(string)
				if name == "" {
					continue
				}
				publicName := "mcp__" + serverName + "__" + name
				if enforceConfiguredTools {
					if _, ok := configuredToolsSet[publicName]; !ok {
						continue
					}
				}
				if enforceAllow {
					if _, ok := allowedSet[publicName]; !ok {
						continue
					}
				}
				if _, blocked := disallowedSet[publicName]; blocked {
					continue
				}

				desc, _ := t["description"].(string)
				schema := map[string]any{"type": "object", "properties": map[string]any{}}
				if raw, ok := t["inputSchema"]; ok {
					schema = toMap(raw)
				}
				r.byName[publicName] = ToolRef{
					Server: serverName,
					Tool:   name,
					Desc:   desc,
					Schema: schema,
					Kind:   "sdk",
				}
			}

		case *mcp.StdioServerConfig, *mcp.SSEServerConfig, *mcp.HTTPServerConfig:
			tools, err := externalListTools(cfg)
			if err != nil {
				status.Status = "error"
				status.Reason = err.Error()
				r.serverStatus[serverName] = status
				continue
			}
			status.Status = "ready"
			r.serverStatus[serverName] = status
			for _, t := range tools {
				if t.Name == "" {
					continue
				}
				publicName := "mcp__" + serverName + "__" + t.Name
				if enforceConfiguredTools {
					if _, ok := configuredToolsSet[publicName]; !ok {
						continue
					}
				}
				if enforceAllow {
					if _, ok := allowedSet[publicName]; !ok {
						continue
					}
				}
				if _, blocked := disallowedSet[publicName]; blocked {
					continue
				}
				schema := t.Schema
				if schema == nil {
					schema = map[string]any{"type": "object", "properties": map[string]any{}}
				}
				r.byName[publicName] = ToolRef{
					Server: serverName,
					Tool:   t.Name,
					Desc:   t.Desc,
					Schema: schema,
					Kind:   "external",
				}
			}
		default:
			status.Status = "error"
			status.Reason = "unknown server type"
			r.serverStatus[serverName] = status
		}
	}
	return r
}

// OpenAITools returns OpenAI-compatible tool definitions.
func (r *Registry) OpenAITools() []map[string]any {
	if r == nil || len(r.byName) == 0 {
		return nil
	}
	names := make([]string, 0, len(r.byName))
	for n := range r.byName {
		names = append(names, n)
	}
	sort.Strings(names)

	out := make([]map[string]any, 0, len(names))
	for _, name := range names {
		ref := r.byName[name]
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        name,
				"description": ref.Desc,
				"parameters":  ref.Schema,
			},
		})
	}
	return out
}

// Get resolves a tool reference by public name.
func (r *Registry) Get(name string) (ToolRef, bool) {
	if r == nil {
		return ToolRef{}, false
	}
	ref, ok := r.byName[name]
	return ref, ok
}

// ResolveName returns a registered tool name that matches `name` under a
// loose equivalence — ignoring runs of underscores. LM Studio thinking-mode
// models (e.g. Qwen3) sometimes emit `mcp_server_tool` in their Hermes XML
// output where the canonical name is `mcp__server__tool`. Returns the
// canonical name and true on a unique match, else empty + false.
func (r *Registry) ResolveName(name string) (string, bool) {
	if r == nil || name == "" {
		return "", false
	}
	if _, ok := r.byName[name]; ok {
		return name, true
	}
	needle := collapseUnderscores(name)
	var matches []string
	for registered := range r.byName {
		if collapseUnderscores(registered) == needle {
			matches = append(matches, registered)
		}
	}
	if len(matches) == 1 {
		return matches[0], true
	}
	return "", false
}

func collapseUnderscores(s string) string {
	if !strings.Contains(s, "_") {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	prev := rune(0)
	for _, r := range s {
		if r == '_' && prev == '_' {
			continue
		}
		b.WriteRune(r)
		prev = r
	}
	return b.String()
}

// ServerStatuses returns the current MCP server discovery status by server name.
func (r *Registry) ServerStatuses() map[string]mcp.ServerStatus {
	if r == nil || len(r.serverStatus) == 0 {
		return map[string]mcp.ServerStatus{}
	}
	out := make(map[string]mcp.ServerStatus, len(r.serverStatus))
	for k, v := range r.serverStatus {
		out[k] = v
	}
	return out
}

func toSet(in []string) map[string]struct{} {
	if len(in) == 0 {
		return map[string]struct{}{}
	}
	m := make(map[string]struct{}, len(in))
	for _, v := range in {
		m[strings.TrimSpace(v)] = struct{}{}
	}
	return m
}

func toConfiguredToolsSet(tc config.ToolsConfig) (map[string]struct{}, bool) {
	switch v := tc.(type) {
	case config.ToolsList:
		out := toSet([]string(v))
		return out, len(out) > 0
	case *config.ToolsList:
		if v == nil {
			return map[string]struct{}{}, false
		}
		out := toSet([]string(*v))
		return out, len(out) > 0
	default:
		return map[string]struct{}{}, false
	}
}

func toMap(v any) map[string]any {
	if v == nil {
		return map[string]any{}
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	b, err := json.Marshal(v)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if json.Unmarshal(b, &out) != nil {
		return map[string]any{}
	}
	return out
}
