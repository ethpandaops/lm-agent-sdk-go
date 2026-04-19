package main

import "testing"

func TestResolveVerifyModel_Default(t *testing.T) {
	t.Setenv("LM_MODEL", "")

	if got := resolveVerifyModel(""); got != defaultVerifyModel {
		t.Fatalf("expected default model %q, got %q", defaultVerifyModel, got)
	}
}

func TestResolveVerifyModel_UsesEnvironmentOverride(t *testing.T) {
	t.Setenv("LM_MODEL", "Qwen/Qwen2.5-7B-Instruct")

	if got := resolveVerifyModel(""); got != "Qwen/Qwen2.5-7B-Instruct" {
		t.Fatalf("expected env override, got %q", got)
	}
}

func TestResolveVerifyModel_FlagWins(t *testing.T) {
	t.Setenv("LM_MODEL", "Qwen/Qwen2.5-3B-Instruct")

	if got := resolveVerifyModel("Qwen/Qwen2.5-14B-Instruct"); got != "Qwen/Qwen2.5-14B-Instruct" {
		t.Fatalf("expected flag override, got %q", got)
	}
}

func TestParseVerdictText_RawJSON(t *testing.T) {
	got, ok := parseVerdictText(`{"pass":true,"reason":"looks good"}`)
	if !ok {
		t.Fatal("expected raw JSON verdict to parse")
	}
	if !got.Pass || got.Reason != "looks good" {
		t.Fatalf("unexpected verdict: %#v", got)
	}
}

func TestParseVerdictText_FencedJSON(t *testing.T) {
	got, ok := parseVerdictText("```json\n{\"pass\":false,\"reason\":\"expected failure\"}\n```")
	if !ok {
		t.Fatal("expected fenced JSON verdict to parse")
	}
	if got.Pass || got.Reason != "expected failure" {
		t.Fatalf("unexpected verdict: %#v", got)
	}
}

func TestParseVerdictText_JSONEmbeddedInProse(t *testing.T) {
	got, ok := parseVerdictText("Here is the result:\n{\"pass\":true,\"reason\":\"output matches\"}\nThanks.")
	if !ok {
		t.Fatal("expected embedded JSON verdict to parse")
	}
	if !got.Pass || got.Reason != "output matches" {
		t.Fatalf("unexpected verdict: %#v", got)
	}
}

func TestShortcutVerdict_Interrupt(t *testing.T) {
	got, ok := shortcutVerdict("interrupt", "Interrupt requested.\nInterrupt completed as expected.\n")
	if !ok {
		t.Fatal("expected interrupt shortcut verdict")
	}
	if !got.Pass {
		t.Fatalf("unexpected interrupt verdict: %#v", got)
	}
}
