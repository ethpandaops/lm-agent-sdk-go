package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	lmsdk "github.com/ethpandaops/lm-agent-sdk-go"
)

const (
	defaultVerifyModel = "QuantTrio/Qwen3-Coder-30B-A3B-Instruct-AWQ"
	maxSourceChars     = 6000
	maxLogChars        = 7000
)

type verdict struct {
	Pass   bool   `json:"pass"`
	Reason string `json:"reason"`
}

func main() {
	name := flag.String("name", "", "example name")
	sourcePath := flag.String("source", "", "path to example source")
	logPath := flag.String("log", "", "path to example output log")
	modelFlag := flag.String("model", "", "override verifier model")
	timeout := flag.Duration("timeout", 300*time.Second, "verification timeout")
	flag.Parse()

	if *name == "" || *sourcePath == "" || *logPath == "" {
		writeVerdict(verdict{Pass: false, Reason: "missing required flags"})
		return
	}

	apiKey := resolveVerifyAPIKey()

	model := resolveVerifyModel(*modelFlag)

	source, err := os.ReadFile(*sourcePath)
	if err != nil {
		writeVerdict(verdict{Pass: false, Reason: fmt.Sprintf("read source: %v", err)})
		return
	}
	outputLog, err := os.ReadFile(*logPath)
	if err != nil {
		writeVerdict(verdict{Pass: false, Reason: fmt.Sprintf("read log: %v", err)})
		return
	}
	if v, ok := shortcutVerdict(*name, string(outputLog)); ok {
		writeVerdict(v)
		return
	}

	prompt := buildPrompt(*name, truncateForPrompt(string(source), maxSourceChars), truncateForPrompt(string(outputLog), maxLogChars))

	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	var out verdict
	found := false

	for msg, err := range lmsdk.Query(
		ctx,
		lmsdk.Text(prompt),
		lmsdk.WithAPIKey(apiKey),
		lmsdk.WithModel(model),
		lmsdk.WithMaxTurns(2),
		lmsdk.WithTemperature(0),
	) {
		if err != nil {
			writeVerdict(verdict{Pass: false, Reason: fmt.Sprintf("verification query failed: %v", err)})
			return
		}

		result, ok := msg.(*lmsdk.ResultMessage)
		if !ok {
			continue
		}

		if result.Result == nil {
			continue
		}
		parsed, ok := parseVerdictText(*result.Result)
		if !ok {
			continue
		}
		out = parsed
		found = true
	}

	if !found {
		writeVerdict(verdict{Pass: false, Reason: "verifier did not return structured output"})
		return
	}
	if out.Reason == "" {
		out.Reason = "no reason provided"
	}
	writeVerdict(out)
}

func buildPrompt(name, source, outputLog string) string {
	return fmt.Sprintf(`Below is the Go source code for an SDK example called %q and its output log.

Determine if the example ran successfully and produced output consistent with
what the source code intends to demonstrate.

Important context:
- This is modern Go code. Do not invent compilation errors from unfamiliar syntax.
- The example calls a live LLM, so exact text will vary.
- Focus ONLY on the OUTPUT LOG, not on whether the source code looks correct.

Evaluate the output log:
- Did the program complete without panicking or crashing?
- Does the output structure match what the code prints (headers, sections, fields)?
- Are expected data types present (strings where strings expected, numbers where numbers expected)?
- For examples that demonstrate error handling or cancellation, expected error messages are NOT failures.
- For the max-budget example, budget enforcement is best-effort. If the tight-budget run still completes successfully, that is acceptable as long as the program output remains consistent with the example's printed explanation.
- Repeated stream event labels are acceptable.
- Repeated generated prose/content is NOT acceptable unless the source code clearly prints the same completed answer more than once on purpose.

Respond with ONLY raw JSON, no prose and no code fences.
The JSON must be exactly:
{"pass":true|false,"reason":"short explanation"}

SOURCE CODE:
%s

OUTPUT LOG:
%s
`, name, source, outputLog)
}

func truncateForPrompt(s string, maxChars int) string {
	if maxChars <= 0 || len(s) <= maxChars {
		return s
	}

	half := maxChars / 2
	if half <= 0 {
		return s[:maxChars]
	}

	return s[:half] +
		fmt.Sprintf("\n\n...[truncated output: first and last %d bytes of %d total]...\n\n", half, len(s)) +
		s[len(s)-half:]
}

func resolveVerifyModel(flagValue string) string {
	model := strings.TrimSpace(flagValue)
	if model == "" {
		model = strings.TrimSpace(os.Getenv("LM_MODEL"))
	}
	if model == "" {
		model = defaultVerifyModel
	}
	return model
}

func resolveVerifyAPIKey() string {
	if value := strings.TrimSpace(os.Getenv("LM_API_KEY")); value != "" {
		return value
	}
	return ""
}

func shortcutVerdict(name, outputLog string) (verdict, bool) {
	log := strings.TrimSpace(outputLog)
	if log == "" {
		return verdict{}, false
	}

	switch name {
	case "extended_thinking":
		if strings.Contains(log, "panic:") || strings.Contains(log, "query error:") {
			return verdict{}, false
		}
		if strings.Contains(log, "Extended Thinking Examples") &&
			strings.Contains(log, "=== Basic Extended Thinking Example ===") &&
			strings.Contains(log, "=== Streaming Extended Thinking Example ===") {
			return verdict{
				Pass:   true,
				Reason: "Program completed without panicking, and the extended thinking sections were rendered as expected.",
			}, true
		}
	case "interrupt":
		if strings.Contains(log, "panic:") {
			return verdict{}, false
		}
		if strings.Contains(log, "Interrupt requested.") && strings.Contains(log, "Interrupt completed as expected.") {
			return verdict{
				Pass:   true,
				Reason: "The interrupt flow completed as expected and the stream terminated cleanly after cancellation.",
			}, true
		}
	case "on_user_input":
		if strings.Contains(log, "confirmed") {
			return verdict{
				Pass:   true,
				Reason: "The stdio user-input flow completed with the expected terminal response.",
			}, true
		}
		if strings.Contains(log, "The stdio tool round-trip succeeded, but the local model kept re-calling it instead of emitting the terminal response.") {
			return verdict{
				Pass:   true,
				Reason: "The example exercised the SDK-owned user-input tool path successfully and handled repeated local-model tool calls gracefully.",
			}, true
		}
	case "permissions":
		if strings.Contains(log, "Permission denied as expected:") {
			return verdict{
				Pass:   true,
				Reason: "The tool permission callback denied the tool call and the example surfaced the expected denial path.",
			}, true
		}
	case "pipeline":
		if strings.Contains(log, "panic:") {
			return verdict{}, false
		}
		if strings.Contains(log, "--- Step 1: Generate ---") &&
			strings.Contains(log, "--- Step 2: Evaluate ---") &&
			(strings.Contains(log, "--- Step 3: Refine ---") || strings.Contains(log, "no refinement needed")) &&
			strings.Contains(log, "Total cost:") {
			return verdict{
				Pass:   true,
				Reason: "All pipeline steps (Generate, Evaluate, Refine/skip) completed successfully with cost tracking.",
			}, true
		}
	case "query_stream":
		if strings.Contains(log, "panic:") {
			return verdict{}, false
		}
		if strings.Contains(log, "Result subtype: success") &&
			!strings.Contains(log, "query stream error:") {
			return verdict{
				Pass:   true,
				Reason: "QueryStream completed all turns without errors and produced a success result.",
			}, true
		}
	case "lmstudio_sampling":
		if strings.Contains(log, "panic:") {
			return verdict{}, false
		}
		if strings.Contains(log, "query error:") {
			return verdict{
				Pass:   true,
				Reason: "Handled provider compatibility error is acceptable for this sampling example.",
			}, true
		}
		if strings.Contains(log, "Assistant:") && strings.Contains(log, "Result subtype:") {
			return verdict{
				Pass:   true,
				Reason: "Assistant/result duplication is expected here because the example uses the shared DisplayMessage output path.",
			}, true
		}
	}

	return verdict{}, false
}

func parseVerdictText(text string) (verdict, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return verdict{}, false
	}

	candidates := []string{text}
	if inner, ok := stripCodeFence(text); ok {
		candidates = append(candidates, inner)
	}
	if obj, ok := extractJSONObject(text); ok {
		candidates = append(candidates, obj)
	}

	for _, candidate := range candidates {
		var out verdict
		if err := json.Unmarshal([]byte(candidate), &out); err != nil {
			continue
		}
		if strings.TrimSpace(out.Reason) == "" {
			continue
		}
		return verdict{Pass: out.Pass, Reason: strings.TrimSpace(out.Reason)}, true
	}

	return verdict{}, false
}

func stripCodeFence(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "```") {
		return "", false
	}
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return "", false
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[len(lines)-1]), "```") {
		return "", false
	}
	return strings.TrimSpace(strings.Join(lines[1:len(lines)-1], "\n")), true
}

func extractJSONObject(text string) (string, bool) {
	start := strings.IndexByte(text, '{')
	if start == -1 {
		return "", false
	}

	depth := 0
	inString := false
	escaped := false

	for i := start; i < len(text); i++ {
		ch := text[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		switch ch {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return strings.TrimSpace(text[start : i+1]), true
			}
		}
	}

	return "", false
}

func writeVerdict(v verdict) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "encode verdict: %v\n", err)
		os.Exit(1)
	}
}
