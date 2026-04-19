package lmsdk

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
)

func loadMCPConfig(configValue string) (map[string]mcp.ServerConfig, error) {
	raw, err := readMCPConfigBytes(configValue)
	if err != nil {
		return nil, err
	}
	return decodeMCPConfig(raw)
}

func readMCPConfigBytes(configValue string) ([]byte, error) {
	trimmed := strings.TrimSpace(configValue)
	if trimmed == "" {
		return nil, nil
	}
	if strings.HasPrefix(trimmed, "{") {
		return []byte(trimmed), nil
	}
	b, err := os.ReadFile(trimmed)
	if err != nil {
		return nil, fmt.Errorf("read mcp config %q: %w", trimmed, err)
	}
	return b, nil
}

func decodeMCPConfig(raw []byte) (map[string]mcp.ServerConfig, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("parse MCP config JSON: %w", err)
	}

	serversMap := root
	if nested, ok := root["mcpServers"].(map[string]any); ok && nested != nil {
		serversMap = nested
	}

	out := make(map[string]mcp.ServerConfig, len(serversMap))
	for name, rawCfg := range serversMap {
		cfgMap, ok := rawCfg.(map[string]any)
		if !ok {
			continue
		}
		cfg, err := decodeSingleMCPServer(name, cfgMap)
		if err != nil {
			return nil, err
		}
		out[name] = cfg
	}
	return out, nil
}

func decodeSingleMCPServer(name string, cfg map[string]any) (mcp.ServerConfig, error) {
	serverType, _ := cfg["type"].(string)
	serverType = strings.ToLower(strings.TrimSpace(serverType))

	if serverType == "" {
		if _, ok := cfg["command"].(string); ok {
			serverType = string(mcp.ServerTypeStdio)
		} else if _, ok := cfg["url"].(string); ok {
			serverType = string(mcp.ServerTypeHTTP)
		}
	}

	switch mcp.ServerType(serverType) {
	case mcp.ServerTypeStdio:
		command, _ := cfg["command"].(string)
		args := stringSliceFromAny(cfg["args"])
		env := stringMapFromAny(cfg["env"])
		t := mcp.ServerTypeStdio
		return &mcp.StdioServerConfig{
			Type:    &t,
			Command: command,
			Args:    args,
			Env:     env,
		}, nil
	case mcp.ServerTypeSSE:
		url, _ := cfg["url"].(string)
		return &mcp.SSEServerConfig{
			Type:    mcp.ServerTypeSSE,
			URL:     url,
			Headers: stringMapFromAny(cfg["headers"]),
		}, nil
	case mcp.ServerTypeHTTP:
		url, _ := cfg["url"].(string)
		return &mcp.HTTPServerConfig{
			Type:    mcp.ServerTypeHTTP,
			URL:     url,
			Headers: stringMapFromAny(cfg["headers"]),
		}, nil
	case mcp.ServerTypeSDK:
		return nil, fmt.Errorf("mcp config server %q type sdk is unsupported in config loading", name)
	default:
		return nil, fmt.Errorf("mcp config server %q has unsupported type %q", name, serverType)
	}
}

func stringSliceFromAny(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func stringMapFromAny(v any) map[string]string {
	raw, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(raw))
	for k, val := range raw {
		if s, ok := val.(string); ok {
			out[k] = s
		}
	}
	return out
}
