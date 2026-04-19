package lmsdk

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMCPConfigRawJSON(t *testing.T) {
	cfg, err := loadMCPConfig(`{"mcpServers":{"ext":{"type":"http","url":"http://example.com"}}}`)
	if err != nil {
		t.Fatalf("load mcp config: %v", err)
	}
	if _, ok := cfg["ext"]; !ok {
		t.Fatalf("expected ext server")
	}
}

func TestLoadMCPConfigFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "mcp.json")
	if err := os.WriteFile(path, []byte(`{"mcpServers":{"ext":{"type":"sse","url":"http://example.com/sse"}}}`), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := loadMCPConfig(path)
	if err != nil {
		t.Fatalf("load mcp config: %v", err)
	}
	if _, ok := cfg["ext"]; !ok {
		t.Fatalf("expected ext server")
	}
}

func TestLoadMCPConfigRejectsSDKType(t *testing.T) {
	_, err := loadMCPConfig(`{"mcpServers":{"ext":{"type":"sdk"}}}`)
	if err == nil {
		t.Fatalf("expected error")
	}
}
