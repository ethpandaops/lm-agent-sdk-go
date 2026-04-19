package lmstudio

import "testing"

func TestLooksReasoningModel(t *testing.T) {
	tests := map[string]bool{
		// DeepSeek R1 family
		"deepseek-r1":                      true,
		"deepseek-r1-distill-llama-70b":    true,
		"deepseek-r1-distill-qwen-14b-awq": true,

		// Explicit naming
		"mistralai/ministral-3-14b-reasoning": true,
		"community/reasoning-tuned-v2":        true,

		// Thinking-mode builds
		"phi-4-thinking":                 true,
		"microsoft/phi-4-thinking-14b":   true,
		"qwen/qwen3-thinking-plus-test":  true,
		"unsloth/qwen3-8b-thinking-2507": true,

		// Qwen3+ reasoning defaults
		"qwen/qwen3.6-35b-a3b":    true,
		"qwen/qwen3.5-9b":         true,
		"qwen/qwen3-coder-30b":    true,
		"qwen/qwen4.0-7b-preview": true,
		"qwen/qwen5":              true,

		// OpenAI-style reasoning tokens (whole-word)
		"o1-mini":                     true,
		"openai/o1":                   true,
		"openai/o3-mini":              true,
		"mycompany/o4_high":           true,
		"marco-o1":                    true,
		"community/gpt-oss-reasoning": true,

		// Negative cases
		"meta-llama/llama-3.1-8b-instruct":     false,
		"mistralai/mistral-7b-instruct":        false,
		"qwen/qwen2.5-7b-instruct":             false,
		"qwen/qwen2-vl-7b":                     false,
		"text-embedding-nomic-embed-text-v1.5": false,
		"zai-org/glm-4.6v-flash":               false,
		"":                                     false,
		// False-positive guards on the short OpenAI tokens
		"foo1-bar":     false, // "o1" not whole-word
		"storio3-base": false, // "o3" not whole-word
		"micro4":       false, // "o4" not whole-word
	}

	for name, want := range tests {
		if got := looksReasoningModel(name); got != want {
			t.Errorf("looksReasoningModel(%q) = %v, want %v", name, got, want)
		}
	}
}
