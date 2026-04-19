package lmsdk

import (
	"context"
	"encoding/json"

	sdkerrors "github.com/ethpandaops/lm-agent-sdk-go/internal/errors"
)

// ListSessions returns local SDK sessions from the configured session store.
func ListSessions(ctx context.Context, opts ...Option) ([]SessionStat, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manager, err := loadSessionManager(opts...)
	if err != nil {
		return nil, err
	}

	sessions := manager.List()
	out := make([]SessionStat, 0, len(sessions))
	for _, s := range sessions {
		out = append(out, *buildSessionStat(s))
	}
	return out, nil
}

// GetSessionMessages returns persisted local session messages.
func GetSessionMessages(ctx context.Context, sessionID string, opts ...Option) ([]Message, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	manager, err := loadSessionManager(opts...)
	if err != nil {
		return nil, err
	}

	s, ok := manager.Get(sessionID)
	if !ok {
		return nil, sdkerrors.ErrSessionNotFound
	}

	out := make([]Message, 0, len(s.Messages))
	for _, raw := range s.Messages {
		msg, err := persistedSessionMessage(raw)
		if err != nil {
			return nil, err
		}
		if msg != nil {
			out = append(out, msg)
		}
	}
	return out, nil
}

func persistedSessionMessage(raw map[string]any) (Message, error) {
	role, _ := raw["role"].(string)
	switch role {
	case "system":
		return &SystemMessage{
			Type:    "system",
			Subtype: "persisted",
			Data:    map[string]any{"content": raw["content"]},
		}, nil
	case "user":
		return &UserMessage{
			Type:    "user",
			Content: parseUserContentValue(raw["content"]),
		}, nil
	case "assistant":
		blocks := make([]ContentBlock, 0, 4)
		if text := stringValue(raw["content"]); text != "" {
			blocks = append(blocks, &TextBlock{Type: "text", Text: text})
		}
		for _, image := range normalizeImages(raw["images"]) {
			blocks = append(blocks, &ImageBlock{
				Type:      "image",
				URL:       imageURL(image),
				MediaType: stringValue(image["media_type"]),
			})
		}
		for _, call := range normalizeToolCalls(raw["tool_calls"]) {
			function := mapValue(call["function"])
			blocks = append(blocks, &ToolUseBlock{
				Type:  "tool_use",
				ID:    stringValue(call["id"]),
				Name:  stringValue(function["name"]),
				Input: parseArgumentsValue(function["arguments"]),
			})
		}
		if len(blocks) > 0 {
			return &AssistantMessage{Type: "assistant", Content: blocks}, nil
		}
		return &AssistantMessage{
			Type: "assistant",
			Content: []ContentBlock{
				&TextBlock{Type: "text", Text: stringValue(raw["content"])},
			},
		}, nil
	case "tool":
		return &AssistantMessage{
			Type: "assistant",
			Content: []ContentBlock{
				&ToolResultBlock{
					Type:      "tool_result",
					ToolUseID: stringValue(raw["tool_call_id"]),
					Content: []ContentBlock{
						&TextBlock{Type: "text", Text: stringValue(raw["content"])},
					},
				},
			},
		}, nil
	default:
		return nil, nil
	}
}

func mapValue(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

func normalizeToolCalls(v any) []map[string]any {
	switch calls := v.(type) {
	case []map[string]any:
		return calls
	case []any:
		out := make([]map[string]any, 0, len(calls))
		for _, raw := range calls {
			if m, ok := raw.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func normalizeImages(v any) []map[string]any {
	switch images := v.(type) {
	case []map[string]any:
		return images
	case []any:
		out := make([]map[string]any, 0, len(images))
		for _, raw := range images {
			if m, ok := raw.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	default:
		return nil
	}
}

func imageURL(raw map[string]any) string {
	if nested, ok := raw["image_url"].(map[string]any); ok {
		return stringValue(nested["url"])
	}
	return stringValue(raw["url"])
}

func parseArgumentsValue(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return map[string]any{}
	}
	return out
}

func parseUserContentValue(v any) UserMessageContent {
	switch raw := v.(type) {
	case string:
		return NewUserMessageContent(raw)
	default:
		data, err := json.Marshal(raw)
		if err != nil {
			return NewUserMessageContent("")
		}
		var content UserMessageContent
		if err := json.Unmarshal(data, &content); err != nil {
			return NewUserMessageContent("")
		}
		return content
	}
}
