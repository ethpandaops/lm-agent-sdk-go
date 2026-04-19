package lmstudio

import (
	"os"
	"strings"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/config"
)

// ResolveAPIKey resolves an optional bearer token for LM Studio servers.
func ResolveAPIKey(opts *config.Options) string {
	if opts != nil && strings.TrimSpace(opts.APIKey) != "" {
		return strings.TrimSpace(opts.APIKey)
	}
	if value := strings.TrimSpace(os.Getenv("LM_API_KEY")); value != "" {
		return value
	}
	return ""
}

// ResolveBaseURL resolves the LM Studio base URL from explicit options or environment.
func ResolveBaseURL(opts *config.Options) string {
	if opts != nil && strings.TrimSpace(opts.BaseURL) != "" {
		return strings.TrimSpace(opts.BaseURL)
	}
	if value := strings.TrimSpace(os.Getenv("LM_BASE_URL")); value != "" {
		return value
	}
	return config.DefaultBaseURL
}
