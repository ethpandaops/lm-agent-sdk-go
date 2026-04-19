// Package main demonstrates a filesystem-backed memory tool for agent state persistence.
//
// This example implements a memory MCP server that allows the model to store
// and retrieve information, similar to a persistent key-value store.
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

// MemoryStore provides filesystem-backed persistent storage.
// All paths must be prefixed with /memories to prevent directory traversal.
type MemoryStore struct {
	basePath string
}

// NewMemoryStore creates a new memory store at the specified base path.
func NewMemoryStore(basePath string) (*MemoryStore, error) {
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base path: %w", err)
	}

	memoriesPath := filepath.Join(absPath, "memories")
	if err := os.MkdirAll(memoriesPath, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create memories directory: %w", err)
	}

	return &MemoryStore{basePath: absPath}, nil
}

func (m *MemoryStore) validatePath(path string) (string, error) {
	if !strings.HasPrefix(path, "/memories") {
		return "", fmt.Errorf("path must start with /memories, got: %s", path)
	}

	relativePath := strings.TrimPrefix(path, "/")
	fullPath := filepath.Join(m.basePath, relativePath)

	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to resolve path: %w", err)
	}

	if !strings.HasPrefix(absPath, m.basePath) {
		return "", fmt.Errorf("path escapes base directory: %s", path)
	}

	return absPath, nil
}

func (m *MemoryStore) Read(path string) (string, error) {
	fullPath, err := m.validatePath(path)
	if err != nil {
		return "", err
	}

	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("file not found: %s", path)
		}
		return "", fmt.Errorf("failed to stat path: %w", err)
	}

	if info.IsDir() {
		dirEntries, readErr := os.ReadDir(fullPath)
		if readErr != nil {
			return "", fmt.Errorf("failed to read directory: %w", readErr)
		}

		var names []string
		for _, entry := range dirEntries {
			name := entry.Name()
			if entry.IsDir() {
				name += "/"
			}
			names = append(names, name)
		}

		return strings.Join(names, "\n"), nil
	}

	content, err := os.ReadFile(fullPath)
	if err != nil {
		return "", fmt.Errorf("failed to read file: %w", err)
	}

	return string(content), nil
}

func (m *MemoryStore) Write(path, content string) error {
	fullPath, err := m.validatePath(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

func (m *MemoryStore) List(path string) ([]string, error) {
	fullPath, err := m.validatePath(path)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(fullPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() {
			name += "/"
		}
		names = append(names, name)
	}

	return names, nil
}

func createMemoryTools(store *MemoryStore) []*sdk.SdkMcpTool {
	readTool := sdk.NewSdkMcpTool(
		"read", "Read file contents or list directory. Path must start with /memories",
		sdk.SimpleSchema(map[string]string{"path": "string"}),
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			args, err := sdk.ParseArguments(req)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			path, _ := args["path"].(string)
			content, err := store.Read(path)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			return sdk.TextResult(content), nil
		},
	)

	writeTool := sdk.NewSdkMcpTool(
		"write", "Create or overwrite a file. Path must start with /memories",
		sdk.SimpleSchema(map[string]string{"path": "string", "content": "string"}),
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			args, err := sdk.ParseArguments(req)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			path, _ := args["path"].(string)
			content, _ := args["content"].(string)
			if err := store.Write(path, content); err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			return sdk.TextResult(fmt.Sprintf("Successfully wrote to %s", path)), nil
		},
	)

	listTool := sdk.NewSdkMcpTool(
		"list", "List directory contents. Path must start with /memories",
		sdk.SimpleSchema(map[string]string{"path": "string"}),
		func(_ context.Context, req *sdk.CallToolRequest) (*sdk.CallToolResult, error) {
			args, err := sdk.ParseArguments(req)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			path, _ := args["path"].(string)
			entries, err := store.List(path)
			if err != nil {
				return sdk.ErrorResult(err.Error()), nil
			}
			if len(entries) == 0 {
				return sdk.TextResult("Directory is empty"), nil
			}
			return sdk.TextResult(strings.Join(entries, "\n")), nil
		},
	)

	return []*sdk.SdkMcpTool{readTool, writeTool, listTool}
}

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	fmt.Println("Memory Tool Example")
	fmt.Println("Demonstrating filesystem-backed persistent memory")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	tmpDir, err := os.MkdirTemp("", "lm-memory-*")
	if err != nil {
		fmt.Printf("Failed to create temp dir: %v\n", err)
		return
	}
	defer func() { _ = os.RemoveAll(tmpDir) }()

	store, err := NewMemoryStore(tmpDir)
	if err != nil {
		fmt.Printf("Failed to create memory store: %v\n", err)
		return
	}

	fmt.Printf("Memory store created at %s/memories/\n\n", tmpDir)

	tools := createMemoryTools(store)
	memoryServer := sdk.CreateSdkMcpServer("memory", "1.0.0", tools...)

	client := sdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()
	defer func() { _ = client.Close() }()

	if err := client.Start(ctx,
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithMCPServers(map[string]sdk.MCPServerConfig{
			"memory": memoryServer,
		}),
		sdk.WithAllowedTools(
			"mcp__memory__read",
			"mcp__memory__write",
			"mcp__memory__list",
		),
		sdk.WithMaxTurns(10),
		sdk.WithSystemPrompt("You have access to memory tools that persist data to the filesystem. "+
			"All paths must start with /memories. Use these tools to remember and recall information."),
	); err != nil {
		fmt.Printf("start error: %v\n", err)
		return
	}

	prompts := []string{
		"Please remember that my name is Alice and my favorite color is blue. " +
			"Store this in /memories/user_info.txt",
		"What is my name and favorite color? Read from the memory you just stored.",
		"List all files in the /memories directory.",
	}

	for i, prompt := range prompts {
		fmt.Printf("\n--- Query %d ---\n", i+1)
		fmt.Printf("Prompt: %s\n", prompt)
		fmt.Println(strings.Repeat("-", 50))

		if err := client.Query(ctx, sdk.Text(prompt)); err != nil {
			fmt.Printf("query error: %v\n", err)
			return
		}

		for msg, err := range client.ReceiveResponse(ctx) {
			if err != nil {
				fmt.Printf("receive error: %v\n", err)
				return
			}
			exampleutil.DisplayMessage(msg)
		}
	}

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("Files persisted in memory store:")

	entries, listErr := os.ReadDir(filepath.Join(tmpDir, "memories"))
	if listErr != nil {
		fmt.Printf("Failed to list directory: %v\n", listErr)
		return
	}

	for _, entry := range entries {
		entryPath := filepath.Join(tmpDir, "memories", entry.Name())
		if !entry.IsDir() {
			content, readErr := os.ReadFile(entryPath)
			if readErr != nil {
				fmt.Printf("  %s: (error reading)\n", entry.Name())
				continue
			}
			fmt.Printf("  %s: %s\n", entry.Name(), strings.TrimSpace(string(content)))
		} else {
			fmt.Printf("  %s/\n", entry.Name())
		}
	}
}
