// Package message provides internal message and content block types.
package message

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"
)

// Block type constants.
const (
	BlockTypeText       = "text"
	BlockTypeImage      = "image"
	BlockTypeImageURL   = "image_url"
	BlockTypeFile       = "file"
	BlockTypeInputAudio = "input_audio"
	BlockTypeVideoURL   = "video_url"
	BlockTypeThinking   = "thinking"
	BlockTypeToolUse    = "tool_use"
	BlockTypeToolResult = "tool_result"
)

// ContentBlock represents a block of content within a message.
type ContentBlock interface {
	BlockType() string
}

// Compile-time verification that all content block types implement ContentBlock.
var (
	_ ContentBlock = (*TextBlock)(nil)
	_ ContentBlock = (*ImageBlock)(nil)
	_ ContentBlock = (*InputImageBlock)(nil)
	_ ContentBlock = (*InputFileBlock)(nil)
	_ ContentBlock = (*InputAudioBlock)(nil)
	_ ContentBlock = (*InputVideoBlock)(nil)
	_ ContentBlock = (*ThinkingBlock)(nil)
	_ ContentBlock = (*ToolUseBlock)(nil)
	_ ContentBlock = (*ToolResultBlock)(nil)
)

// TextBlock contains plain text content.
type TextBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// BlockType implements the ContentBlock interface.
func (b *TextBlock) BlockType() string { return BlockTypeText }

// InputImageRef identifies an input image URL or data URL.
type InputImageRef struct {
	URL string `json:"url"`
}

// InputImageBlock contains an input image reference for multimodal prompts.
type InputImageBlock struct {
	Type     string        `json:"type"`
	ImageURL InputImageRef `json:"image_url"`
}

// BlockType implements the ContentBlock interface.
func (b *InputImageBlock) BlockType() string { return BlockTypeImageURL }

// InputFileRef identifies an input file URL or data URL.
type InputFileRef struct {
	Filename string `json:"filename,omitempty"`
	FileData string `json:"file_data"`
}

// InputFileBlock contains an input file reference for multimodal prompts.
type InputFileBlock struct {
	Type string       `json:"type"`
	File InputFileRef `json:"file"`
}

// BlockType implements the ContentBlock interface.
func (b *InputFileBlock) BlockType() string { return BlockTypeFile }

// InputAudioRef contains base64-encoded input audio plus its format.
type InputAudioRef struct {
	Data   string `json:"data"`
	Format string `json:"format"`
}

// InputAudioBlock contains an input audio payload for multimodal prompts.
type InputAudioBlock struct {
	Type       string        `json:"type"`
	InputAudio InputAudioRef `json:"input_audio"`
}

// BlockType implements the ContentBlock interface.
func (b *InputAudioBlock) BlockType() string { return BlockTypeInputAudio }

// InputVideoRef identifies an input video URL or data URL.
type InputVideoRef struct {
	URL string `json:"url"`
}

// InputVideoBlock contains an input video reference for multimodal prompts.
type InputVideoBlock struct {
	Type     string        `json:"type"`
	VideoURL InputVideoRef `json:"video_url"`
}

// BlockType implements the ContentBlock interface.
func (b *InputVideoBlock) BlockType() string { return BlockTypeVideoURL }

// ImageBlock contains a generated image reference.
type ImageBlock struct {
	Type      string `json:"type"`
	URL       string `json:"url"`
	MediaType string `json:"media_type,omitempty"`
}

// BlockType implements the ContentBlock interface.
func (b *ImageBlock) BlockType() string { return BlockTypeImage }

// Decode returns the raw bytes and media type for data-url-backed image blocks.
func (b *ImageBlock) Decode() ([]byte, string, error) {
	if b == nil {
		return nil, "", fmt.Errorf("image block is nil")
	}
	raw := strings.TrimSpace(b.URL)
	if !strings.HasPrefix(raw, "data:") {
		return nil, "", fmt.Errorf("image URL is not a data URL")
	}
	meta, payload, ok := strings.Cut(raw, ",")
	if !ok {
		return nil, "", fmt.Errorf("invalid data URL")
	}
	if !strings.HasSuffix(meta, ";base64") {
		return nil, "", fmt.Errorf("data URL is not base64-encoded")
	}
	mediaType := strings.TrimPrefix(strings.TrimSuffix(meta, ";base64"), "data:")
	if mediaType == "" {
		mediaType = b.MediaType
	}
	data, err := base64.StdEncoding.DecodeString(payload)
	if err != nil {
		return nil, "", fmt.Errorf("decode image data: %w", err)
	}
	return data, mediaType, nil
}

// FileExtension returns a best-effort file extension for the image.
func (b *ImageBlock) FileExtension() string {
	if b == nil {
		return ".bin"
	}
	mediaType := strings.TrimSpace(b.MediaType)
	if mediaType == "" {
		if _, parsedMediaType, err := b.Decode(); err == nil {
			mediaType = parsedMediaType
		}
	}
	if mediaType == "" {
		return ".bin"
	}
	exts, err := mime.ExtensionsByType(mediaType)
	if err != nil || len(exts) == 0 {
		switch mediaType {
		case "image/png":
			return ".png"
		case "image/jpeg":
			return ".jpg"
		case "image/webp":
			return ".webp"
		default:
			return ".bin"
		}
	}
	return exts[0]
}

// Save writes a data-url-backed image to disk.
func (b *ImageBlock) Save(path string) error {
	if b == nil {
		return fmt.Errorf("image block is nil")
	}
	data, _, err := b.Decode()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create image directory: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write image file: %w", err)
	}
	return nil
}

// ThinkingBlock contains model reasoning text.
type ThinkingBlock struct {
	Type      string `json:"type"`
	Thinking  string `json:"thinking"`
	Signature string `json:"signature"`
}

// BlockType implements the ContentBlock interface.
func (b *ThinkingBlock) BlockType() string { return BlockTypeThinking }

// ToolUseBlock represents a model tool call.
type ToolUseBlock struct {
	Type  string         `json:"type"`
	ID    string         `json:"id"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// BlockType implements the ContentBlock interface.
func (b *ToolUseBlock) BlockType() string { return BlockTypeToolUse }

// ToolResultBlock contains the result of a tool execution.
//
//nolint:tagliatelle // Legacy compatibility payloads use snake_case fields.
type ToolResultBlock struct {
	Type      string         `json:"type"`
	ToolUseID string         `json:"tool_use_id"`
	Content   []ContentBlock `json:"content,omitempty"`
	IsError   bool           `json:"is_error,omitempty"`
}

// BlockType implements the ContentBlock interface.
func (b *ToolResultBlock) BlockType() string { return BlockTypeToolResult }

// UnmarshalJSON implements json.Unmarshaler for ToolResultBlock.
// Handles both string content and array content.
func (b *ToolResultBlock) UnmarshalJSON(data []byte) error {
	type Alias ToolResultBlock

	aux := &struct {
		Content json.RawMessage `json:"content,omitempty"`
		*Alias
	}{
		Alias: (*Alias)(b),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	if len(aux.Content) == 0 || string(aux.Content) == "null" {
		return nil
	}

	// Try string first
	var text string
	if err := json.Unmarshal(aux.Content, &text); err == nil {
		b.Content = []ContentBlock{&TextBlock{Type: BlockTypeText, Text: text}}

		return nil
	}

	// Try array of blocks
	var rawBlocks []json.RawMessage
	if err := json.Unmarshal(aux.Content, &rawBlocks); err != nil {
		return err
	}

	b.Content = make([]ContentBlock, 0, len(rawBlocks))

	for _, raw := range rawBlocks {
		block, err := UnmarshalContentBlock(raw)
		if err != nil {
			return err
		}

		b.Content = append(b.Content, block)
	}

	return nil
}

// UnmarshalContentBlock unmarshals a single content block from JSON.
func UnmarshalContentBlock(data []byte) (ContentBlock, error) {
	var typeHolder struct {
		Type string `json:"type"`
	}

	if err := json.Unmarshal(data, &typeHolder); err != nil {
		return nil, err
	}

	switch typeHolder.Type {
	case BlockTypeText:
		var block TextBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}

		return &block, nil
	case BlockTypeImage:
		var block ImageBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}

		return &block, nil
	case BlockTypeImageURL:
		var block InputImageBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}

		return &block, nil
	case BlockTypeFile:
		var block InputFileBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}

		return &block, nil
	case BlockTypeInputAudio:
		var block InputAudioBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}

		return &block, nil
	case BlockTypeVideoURL:
		var block InputVideoBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}

		return &block, nil
	case BlockTypeThinking:
		var block ThinkingBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}

		return &block, nil
	case BlockTypeToolUse:
		var block ToolUseBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}

		return &block, nil
	case BlockTypeToolResult:
		var block ToolResultBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}

		return &block, nil
	default:
		// Return a generic text block for unknown types
		var block TextBlock
		if err := json.Unmarshal(data, &block); err != nil {
			return nil, err
		}

		return &block, nil
	}
}
