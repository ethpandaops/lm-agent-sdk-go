package message

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestImageBlockDecodeAndSave(t *testing.T) {
	block := &ImageBlock{
		Type:      BlockTypeImage,
		URL:       "data:image/png;base64,aGVsbG8=",
		MediaType: "image/png",
	}

	data, mediaType, err := block.Decode()
	if err != nil {
		t.Fatalf("decode image block: %v", err)
	}
	if string(data) != "hello" {
		t.Fatalf("unexpected image bytes: %q", string(data))
	}
	if mediaType != "image/png" {
		t.Fatalf("unexpected media type: %q", mediaType)
	}
	if ext := block.FileExtension(); ext != ".png" {
		t.Fatalf("unexpected file extension: %q", ext)
	}

	path := filepath.Join(t.TempDir(), "icon"+block.FileExtension())
	if err := block.Save(path); err != nil {
		t.Fatalf("save image block: %v", err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read saved image: %v", err)
	}
	if string(raw) != "hello" {
		t.Fatalf("unexpected saved bytes: %q", string(raw))
	}
}

func TestUserMessageContentRoundTripsMultimodalBlocks(t *testing.T) {
	content := NewUserMessageContentBlocks([]ContentBlock{
		&TextBlock{Type: BlockTypeText, Text: "Compare these inputs."},
		&InputImageBlock{
			Type:     BlockTypeImageURL,
			ImageURL: InputImageRef{URL: "data:image/png;base64,aGVsbG8="},
		},
		&InputFileBlock{
			Type: BlockTypeFile,
			File: InputFileRef{
				Filename: "spec.pdf",
				FileData: "data:application/pdf;base64,JVBERi0xLjQK",
			},
		},
		&InputAudioBlock{
			Type: BlockTypeInputAudio,
			InputAudio: InputAudioRef{
				Format: "wav",
				Data:   "UklGRg==",
			},
		},
		&InputVideoBlock{
			Type:     BlockTypeVideoURL,
			VideoURL: InputVideoRef{URL: "https://example.com/demo.mp4"},
		},
	})

	raw, err := json.Marshal(content)
	if err != nil {
		t.Fatalf("marshal content: %v", err)
	}

	var decoded UserMessageContent
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal content: %v", err)
	}

	blocks := decoded.Blocks()
	if len(blocks) != 5 {
		t.Fatalf("expected 5 content blocks, got %#v", blocks)
	}
	if _, ok := blocks[0].(*TextBlock); !ok {
		t.Fatalf("expected first block text, got %T", blocks[0])
	}
	if image, ok := blocks[1].(*InputImageBlock); !ok || image.ImageURL.URL == "" {
		t.Fatalf("expected image input block, got %#v", blocks[1])
	}
	if file, ok := blocks[2].(*InputFileBlock); !ok || file.File.Filename != "spec.pdf" {
		t.Fatalf("expected file input block, got %#v", blocks[2])
	}
	if audio, ok := blocks[3].(*InputAudioBlock); !ok || audio.InputAudio.Format != "wav" {
		t.Fatalf("expected audio input block, got %#v", blocks[3])
	}
	if video, ok := blocks[4].(*InputVideoBlock); !ok || video.VideoURL.URL == "" {
		t.Fatalf("expected video input block, got %#v", blocks[4])
	}
}
