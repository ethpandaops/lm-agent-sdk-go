// Package main demonstrates how to use a custom logging library (logrus) with the
// LM Studio SDK. Since the SDK expects *slog.Logger, this example shows how to create
// an adapter that bridges logrus to slog.
//
// This pattern works for any logging library (zap, zerolog, etc.) - just implement
// the slog.Handler interface.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"

	sdk "github.com/ethpandaops/lm-agent-sdk-go"
	"github.com/ethpandaops/lm-agent-sdk-go/examples/internal/exampleutil"
)

// logrusHandler adapts logrus to the slog.Handler interface.
type logrusHandler struct {
	logger *logrus.Logger
	attrs  []slog.Attr
	groups []string
}

func NewLogrusHandler(logger *logrus.Logger) slog.Handler {
	return &logrusHandler{
		logger: logger,
		attrs:  make([]slog.Attr, 0),
		groups: make([]string, 0),
	}
}

func (h *logrusHandler) Enabled(_ context.Context, level slog.Level) bool {
	return h.logger.IsLevelEnabled(slogToLogrusLevel(level))
}

func (h *logrusHandler) Handle(_ context.Context, record slog.Record) error {
	fields := make(logrus.Fields, record.NumAttrs()+len(h.attrs))

	for _, attr := range h.attrs {
		fields[h.buildKey(attr.Key)] = attr.Value.Any()
	}

	record.Attrs(func(attr slog.Attr) bool {
		fields[h.buildKey(attr.Key)] = attr.Value.Any()
		return true
	})

	h.logger.WithFields(fields).Log(slogToLogrusLevel(record.Level), record.Message)

	return nil
}

func (h *logrusHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, len(h.attrs)+len(attrs))
	copy(newAttrs, h.attrs)
	copy(newAttrs[len(h.attrs):], attrs)

	return &logrusHandler{logger: h.logger, attrs: newAttrs, groups: h.groups}
}

func (h *logrusHandler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}

	newGroups := make([]string, len(h.groups)+1)
	copy(newGroups, h.groups)
	newGroups[len(h.groups)] = name

	return &logrusHandler{logger: h.logger, attrs: h.attrs, groups: newGroups}
}

func (h *logrusHandler) buildKey(key string) string {
	if len(h.groups) == 0 {
		return key
	}

	var result strings.Builder
	for _, g := range h.groups {
		result.WriteString(g + ".")
	}

	return result.String() + key
}

func slogToLogrusLevel(level slog.Level) logrus.Level {
	switch {
	case level >= slog.LevelError:
		return logrus.ErrorLevel
	case level >= slog.LevelWarn:
		return logrus.WarnLevel
	case level >= slog.LevelInfo:
		return logrus.InfoLevel
	default:
		return logrus.DebugLevel
	}
}

func displayMessage(log logrus.FieldLogger, msg sdk.Message) {
	switch m := msg.(type) {
	case *sdk.UserMessage:
		for _, block := range m.Content.Blocks() {
			if textBlock, ok := block.(*sdk.TextBlock); ok {
				log.WithField("role", "user").Info(textBlock.Text)
			}
		}
	case *sdk.AssistantMessage:
		for _, block := range m.Content {
			if textBlock, ok := block.(*sdk.TextBlock); ok {
				log.WithField("role", "assistant").Info(textBlock.Text)
			}
		}
	case *sdk.SystemMessage:
		log.WithField("role", "system").Debug("System message received")
	case *sdk.ResultMessage:
		fields := logrus.Fields{"component": "result"}
		if m.TotalCostUSD != nil {
			fields["cost_usd"] = fmt.Sprintf("$%.8f", *m.TotalCostUSD)
		}
		log.WithFields(fields).Info("Query completed")
	}
}

func main() {
	apiKey := strings.TrimSpace(os.Getenv("LM_API_KEY"))
	if apiKey == "" {
		fmt.Println("Set LM_API_KEY to run this example.")
		return
	}

	model := strings.TrimSpace(os.Getenv("LM_MODEL"))
	if model == "" {
		model = exampleutil.DefaultModel()
	}

	// 1. Configure logrus
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		FullTimestamp:   true,
		TimestampFormat: "2006-01-02 15:04:05",
	})
	log.SetLevel(logrus.DebugLevel)

	log.Info("=== Custom Logger (Logrus) Example ===")

	// 2. Create slog.Logger with logrus adapter
	slogLogger := slog.New(NewLogrusHandler(log))

	// 3. Use with SDK
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	log.Info("Running query with logrus-backed logger...")

	for msg, err := range sdk.Query(ctx, sdk.Text("What is 2+2? Answer in one short sentence."),
		sdk.WithAPIKey(apiKey),
		sdk.WithModel(model),
		sdk.WithLogger(slogLogger),
		sdk.WithMaxTurns(1),
	) {
		if err != nil {
			log.WithError(err).Error("Query failed")
			return
		}
		displayMessage(log, msg)
	}

	log.Info("Example complete - all output above was logged through logrus")
}
