package util

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ParseSSE reads server-sent events and emits parsed JSON data objects.
func ParseSSE(ctx context.Context, r io.Reader, out chan<- map[string]any, errs chan<- error) {
	defer close(out)
	defer close(errs)

	scanner := bufio.NewScanner(r)
	// Support large tool-call argument deltas.
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 2*1024*1024)

	var dataLines []string
	flush := func() bool {
		if len(dataLines) == 0 {
			return true
		}
		payload := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		if payload == "[DONE]" {
			return false
		}
		var msg map[string]any
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			err = fmt.Errorf("decode sse json: %w", err)
			select {
			case errs <- err:
			case <-ctx.Done():
			}
			return true
		}
		select {
		case out <- msg:
			return true
		case <-ctx.Done():
			return false
		}
	}

	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
		}
		line := scanner.Text()
		if line == "" {
			if !flush() {
				return
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if len(dataLines) > 0 {
		flush()
	}
	if err := scanner.Err(); err != nil && !errors.Is(err, io.EOF) {
		select {
		case errs <- err:
		case <-ctx.Done():
		}
	}
}
