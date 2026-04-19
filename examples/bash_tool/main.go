// Package main demonstrates a bash/shell execution tool driven by the model.
//
// This gives any tool-capable LM Studio model the ability to run shell
// commands — similar to Claude Code's Bash tool. The tool:
//
//   - Accepts a command string and optional description.
//   - Captures stdout + stderr and returns them alongside the exit code.
//   - Enforces a per-call timeout (default 30s, overridable per request).
//   - Rejects a configurable denylist of obviously-dangerous commands
//     before exec (best-effort — do not rely on this for untrusted input).
//   - Runs every call through a permission callback so the operator can
//     approve/deny/patch each command before it's executed.
//
// Run it with:
//
//	LM_API_KEY=... LM_MODEL=qwen/qwen3.6-35b-a3b go run ./examples/bash_tool
package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

// dangerousSubstrings are cheap syntactic guards against commands that are
// almost never legitimate for an LLM to run autonomously. The list is not
// exhaustive — treat this tool as "sharp" regardless and rely on the
// permission callback for the real gate.
var dangerousSubstrings = []string{
	"rm -rf /",
	"mkfs",
	":(){ :|:& };:",
	"dd if=/dev/",
	"> /dev/sd",
	"shutdown",
	"reboot",
	"halt",
	"sudo rm",
}

func newBashTool(defaultTimeout time.Duration) sdk.Tool {
	return sdk.NewTool(
		"bash",
		"Run a shell command in a non-interactive /bin/sh subprocess and return stdout/stderr/exit_code. Use for file listings, git status, grep, etc. Commands are subject to per-call operator approval.",
		map[string]any{
			"type": "object",
			"properties": map[string]any{
				"command": map[string]any{
					"type":        "string",
					"description": "Shell command to execute. Non-interactive; no stdin.",
				},
				"description": map[string]any{
					"type":        "string",
					"description": "One-line human-readable summary of what the command does, shown to the operator on the approval prompt.",
				},
				"timeout_seconds": map[string]any{
					"type":        "integer",
					"description": "Per-call timeout in seconds. Defaults to 30.",
					"minimum":     1,
					"maximum":     600,
				},
				"cwd": map[string]any{
					"type":        "string",
					"description": "Working directory for the command. Defaults to the SDK's configured Cwd.",
				},
			},
			"required":             []string{"command"},
			"additionalProperties": false,
		},
		func(ctx context.Context, input map[string]any) (map[string]any, error) {
			command, _ := input["command"].(string)
			command = strings.TrimSpace(command)
			if command == "" {
				return map[string]any{"error": "command is required"}, nil
			}
			lowered := strings.ToLower(command)
			for _, bad := range dangerousSubstrings {
				if strings.Contains(lowered, bad) {
					return map[string]any{
						"error": "command rejected by denylist: " + bad,
					}, nil
				}
			}

			timeout := defaultTimeout
			if raw, ok := input["timeout_seconds"].(float64); ok && raw > 0 {
				timeout = time.Duration(raw) * time.Second
			}

			runCtx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()

			cmd := exec.CommandContext(runCtx, "/bin/sh", "-c", command)
			if cwd, _ := input["cwd"].(string); strings.TrimSpace(cwd) != "" {
				cmd.Dir = cwd
			}
			var stdout, stderr bytes.Buffer
			cmd.Stdout = &stdout
			cmd.Stderr = &stderr
			started := time.Now()
			err := cmd.Run()
			duration := time.Since(started)

			exitCode := 0
			if err != nil {
				var ee *exec.ExitError
				switch {
				case errors.Is(runCtx.Err(), context.DeadlineExceeded):
					return map[string]any{
						"command":     command,
						"error":       fmt.Sprintf("command timed out after %s", timeout),
						"exit_code":   -1,
						"duration_ms": duration.Milliseconds(),
						"stdout":      stdout.String(),
						"stderr":      stderr.String(),
					}, nil
				case errors.As(err, &ee):
					exitCode = ee.ExitCode()
				default:
					return map[string]any{
						"command":     command,
						"error":       err.Error(),
						"exit_code":   -1,
						"duration_ms": duration.Milliseconds(),
						"stdout":      stdout.String(),
						"stderr":      stderr.String(),
					}, nil
				}
			}

			return map[string]any{
				"command":     command,
				"exit_code":   exitCode,
				"stdout":      truncate(stdout.String(), 8192),
				"stderr":      truncate(stderr.String(), 4096),
				"duration_ms": duration.Milliseconds(),
			}, nil
		},
	)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + fmt.Sprintf("\n...[truncated %d bytes]", len(s)-max)
}

// approveCommand is a trivial permission hook that prints the command and
// auto-approves it. Replace with an interactive prompt (stdin) or an allow
// list in production.
func approveCommand(_ context.Context, toolName string, input map[string]any, _ *sdk.ToolPermissionContext) (sdk.PermissionResult, error) {
	if toolName != "mcp__sdk__bash" {
		return &sdk.PermissionResultAllow{Behavior: "allow"}, nil
	}
	command, _ := input["command"].(string)
	description, _ := input["description"].(string)
	if description == "" {
		description = "(no description)"
	}
	fmt.Printf("\n>> model wants to run: %s\n   reason: %s\n", command, description)
	// For the example: auto-approve. Swap in stdin prompting for real use.
	return &sdk.PermissionResultAllow{Behavior: "allow"}, nil
}

func main() {
	if err := exampleutil.RequireAPIKey(); err != nil {
		exampleutil.PrintMissingAPIKeyHint()
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	bash := newBashTool(30 * time.Second)

	prompt := "Use the bash tool to list the three most recently modified files under the current directory (excluding .git). Then summarize what they are in one sentence."

	for msg, err := range sdk.Query(ctx,
		sdk.Text(prompt),
		sdk.WithAPIKey(exampleutil.APIKey()),
		sdk.WithModel(exampleutil.DefaultModel()),
		sdk.WithSDKTools(bash),
		sdk.WithCanUseTool(approveCommand),
		sdk.WithSystemPrompt("You have a bash tool. Prefer short, non-interactive, read-only commands. Never run sudo or destructive operations. After running a command, summarize the result briefly."),
		sdk.WithTemperature(0.1),
		sdk.WithMaxToolIterations(4),
	) {
		if err != nil {
			fmt.Printf("\nquery error: %v\n", err)
			return
		}
		if result, ok := msg.(*sdk.ResultMessage); ok && result.Result != nil {
			fmt.Printf("\nAssistant: %s\n", *result.Result)
		}
	}
}
