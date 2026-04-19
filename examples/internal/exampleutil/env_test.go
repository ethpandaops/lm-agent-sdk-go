package exampleutil

import "testing"

func TestDefaultModelFallback(t *testing.T) {
	t.Setenv("LM_MODEL", "")
	got := DefaultModel()
	if got != "QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ" {
		t.Fatalf("unexpected default model: %q", got)
	}
}

func TestDefaultModelOverride(t *testing.T) {
	t.Setenv("LM_MODEL", "Qwen/Qwen2.5-7B-Instruct")
	got := DefaultModel()
	if got != "Qwen/Qwen2.5-7B-Instruct" {
		t.Fatalf("unexpected override model: %q", got)
	}
}

func TestDefaultImageModelFallback(t *testing.T) {
	t.Setenv("VLLM_IMAGE_MODEL", "")
	got := DefaultImageModel()
	if got != "QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ" {
		t.Fatalf("unexpected default image model: %q", got)
	}
}

func TestDefaultImageModelOverride(t *testing.T) {
	t.Setenv("VLLM_IMAGE_MODEL", "Qwen/Qwen2.5-VL-7B-Instruct")
	got := DefaultImageModel()
	if got != "Qwen/Qwen2.5-VL-7B-Instruct" {
		t.Fatalf("unexpected override image model: %q", got)
	}
}
