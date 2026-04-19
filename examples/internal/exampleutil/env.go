package exampleutil

import (
	"context"
	"fmt"
	"os"
	"strings"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
)

const (
	envAPIKeyPrimary  = "LM_API_KEY"
	envModelPrimary   = "LM_MODEL"
	envImageModel     = "VLLM_IMAGE_MODEL"
	envVisionModel    = "VLLM_VISION_MODEL"
	defaultModel      = "QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ"
	defaultImageModel = "QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ"
)

func APIKey() string {
	if value := strings.TrimSpace(os.Getenv(envAPIKeyPrimary)); value != "" {
		return value
	}
	return ""
}

func RequireAPIKey() error {
	return nil
}

func DefaultModel() string {
	if value := strings.TrimSpace(os.Getenv(envModelPrimary)); value != "" {
		return value
	}
	return defaultModel
}

func DefaultImageModel() string {
	if value := strings.TrimSpace(os.Getenv(envImageModel)); value != "" {
		return value
	}
	return defaultImageModel
}

func DefaultVisionModel() string {
	if value := strings.TrimSpace(os.Getenv(envVisionModel)); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv(envImageModel)); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv(envModelPrimary)); value != "" {
		return value
	}
	return defaultImageModel
}

func ServedModelIDs(ctx context.Context) ([]string, error) {
	models, err := sdk.ListModels(ctx, sdk.WithAPIKey(APIKey()))
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(models))
	for _, model := range models {
		if id := strings.TrimSpace(model.ID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func IsServedModel(ctx context.Context, target string) (bool, []string, error) {
	ids, err := ServedModelIDs(ctx)
	if err != nil {
		return false, nil, err
	}
	target = strings.TrimSpace(target)
	for _, id := range ids {
		if id == target {
			return true, ids, nil
		}
	}
	return false, ids, nil
}

func PrintMissingAPIKeyHint() {
	fmt.Printf("API keys are optional for local LM Studio. Set %s if your server enforces bearer auth.\n", envAPIKeyPrimary)
}
