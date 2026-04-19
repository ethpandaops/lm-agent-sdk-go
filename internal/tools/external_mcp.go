package tools

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"

	gomcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/mcp"
	"github.com/ethpandaops/lm-agent-sdk-go/internal/util"
)

type externalListedTool struct {
	Name   string
	Desc   string
	Schema map[string]any
}

var externalListTools = listExternalTools
var externalCallTool = callExternalTool
var externalConnect = connectExternalSession
var externalBackoff = util.Backoff

func listExternalTools(cfg mcp.ServerConfig) ([]externalListedTool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return withExternalRetry(ctx, 3, func(callCtx context.Context) ([]externalListedTool, error) {
		attemptCtx, cancelAttempt := context.WithTimeout(callCtx, 10*time.Second)
		defer cancelAttempt()

		sess, err := externalConnect(attemptCtx, cfg)
		if err != nil {
			return nil, err
		}
		defer func() { _ = sess.Close() }()

		tools := make([]externalListedTool, 0, 8)
		for t, err := range sess.Tools(attemptCtx, &gomcp.ListToolsParams{}) {
			if err != nil {
				return nil, err
			}
			if t == nil || t.Name == "" {
				continue
			}
			schema := map[string]any{"type": "object", "properties": map[string]any{}}
			if m, ok := t.InputSchema.(map[string]any); ok && m != nil {
				schema = m
			}
			tools = append(tools, externalListedTool{
				Name:   t.Name,
				Desc:   t.Description,
				Schema: schema,
			})
		}
		return tools, nil
	})
}

func callExternalTool(ctx context.Context, cfg mcp.ServerConfig, toolName string, input map[string]any) (*gomcp.CallToolResult, error) {
	callCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()

	return withExternalRetry(callCtx, 3, func(attemptCtx context.Context) (*gomcp.CallToolResult, error) {
		perAttemptCtx, cancelAttempt := context.WithTimeout(attemptCtx, 30*time.Second)
		defer cancelAttempt()

		sess, err := externalConnect(perAttemptCtx, cfg)
		if err != nil {
			return nil, err
		}
		defer func() { _ = sess.Close() }()

		return sess.CallTool(perAttemptCtx, &gomcp.CallToolParams{
			Name:      toolName,
			Arguments: input,
		})
	})
}

func withExternalRetry[T any](
	ctx context.Context,
	attempts int,
	call func(context.Context) (T, error),
) (T, error) {
	var zero T
	if attempts <= 0 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		result, err := call(ctx)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if attempt == attempts-1 || !isRetryableExternalError(err) {
			break
		}
		select {
		case <-ctx.Done():
			return zero, ctx.Err()
		case <-time.After(externalBackoff(attempt)):
		}
	}
	return zero, lastErr
}

func isRetryableExternalError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	msg := strings.ToLower(err.Error())
	nonRetrySubstrings := []string{
		"invalid",
		"unsupported",
		"missing",
		"unknown server type",
	}
	for _, s := range nonRetrySubstrings {
		if strings.Contains(msg, s) {
			return false
		}
	}

	retrySubstrings := []string{
		"timeout",
		"tempor",
		"connection",
		"refused",
		"reset",
		"eof",
		"closed",
		"unavailable",
		"i/o",
	}
	for _, s := range retrySubstrings {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

func connectExternalSession(ctx context.Context, cfg mcp.ServerConfig) (*gomcp.ClientSession, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil MCP server config")
	}

	client := gomcp.NewClient(&gomcp.Implementation{
		Name:    "lm-agent-sdk-go",
		Version: "0.1.0",
	}, nil)

	transport, err := externalTransport(cfg)
	if err != nil {
		return nil, err
	}

	return client.Connect(ctx, transport, nil)
}

func externalTransport(cfg mcp.ServerConfig) (gomcp.Transport, error) {
	switch c := cfg.(type) {
	case *mcp.StdioServerConfig:
		if c == nil || c.Command == "" {
			return nil, fmt.Errorf("invalid stdio MCP config")
		}
		cmd := exec.Command(c.Command, c.Args...)
		if len(c.Env) > 0 {
			env := os.Environ()
			for k, v := range c.Env {
				env = append(env, k+"="+v)
			}
			cmd.Env = env
		}
		return &gomcp.CommandTransport{Command: cmd}, nil

	case *mcp.SSEServerConfig:
		if c == nil || c.URL == "" {
			return nil, fmt.Errorf("invalid sse MCP config")
		}
		return &gomcp.SSEClientTransport{
			Endpoint:   c.URL,
			HTTPClient: makeHeaderClient(c.Headers),
		}, nil

	case *mcp.HTTPServerConfig:
		if c == nil || c.URL == "" {
			return nil, fmt.Errorf("invalid http MCP config")
		}
		return &gomcp.StreamableClientTransport{
			Endpoint:   c.URL,
			HTTPClient: makeHeaderClient(c.Headers),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported MCP server type")
	}
}

func makeHeaderClient(headers map[string]string) *http.Client {
	if len(headers) == 0 {
		return http.DefaultClient
	}
	return &http.Client{
		Transport: &headerRoundTripper{
			base:    http.DefaultTransport,
			headers: headers,
		},
	}
}

type headerRoundTripper struct {
	base    http.RoundTripper
	headers map[string]string
}

func (h *headerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	clone := req.Clone(req.Context())
	for k, v := range h.headers {
		clone.Header.Set(k, v)
	}
	base := h.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(clone)
}
