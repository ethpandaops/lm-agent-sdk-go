package lmsdk

import (
	"context"
	"iter"

	"github.com/ethpandaops/lm-agent-sdk-go/internal/message"
)

// Query executes a one-shot query and returns a message iterator.
func Query(
	ctx context.Context,
	content UserMessageContent,
	opts ...Option,
) iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		c := NewClient()
		if err := c.Start(ctx, opts...); err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = c.Close() }()

		if err := c.Query(ctx, content); err != nil {
			yield(nil, err)
			return
		}

		for msg, err := range c.ReceiveResponse(ctx) {
			if !yield(msg, err) {
				return
			}
		}
	}
}

// QueryStream executes a one-shot query from a stream of user messages.
func QueryStream(
	ctx context.Context,
	messages iter.Seq[StreamingMessage],
	opts ...Option,
) iter.Seq2[Message, error] {
	return func(yield func(Message, error) bool) {
		c := NewClient()
		if err := c.StartWithStream(ctx, messages, opts...); err != nil {
			yield(nil, err)
			return
		}
		defer func() { _ = c.Close() }()

		for msg, err := range c.ReceiveMessages(ctx) {
			if !yield(msg, err) {
				return
			}
		}
	}
}

// MessagesFromContent creates a single-message stream from user content.
func MessagesFromContent(content UserMessageContent) iter.Seq[StreamingMessage] {
	return func(yield func(StreamingMessage) bool) {
		m := message.StreamingMessage{
			Type: "user",
			Message: message.StreamingMessageContent{
				Role:    "user",
				Content: content,
			},
		}
		yield(m)
	}
}
