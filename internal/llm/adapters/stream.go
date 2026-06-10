// Package adapters implements provider-specific LLM adapters backed by the
// official Go SDKs.
package adapters

import (
	"context"

	"github.com/nigelpepper/attractor/internal/llm"
)

// synthStream turns a completed Response into an ordered StreamEvent channel.
// It is used by adapters whose first-cut implementation does not yet wire the
// SDK's native token streaming; the agent loop relies only on Complete, so this
// keeps the ProviderAdapter contract satisfied without partial-token plumbing.
func synthStream(ctx context.Context, resp llm.Response, err error) (<-chan llm.StreamEvent, error) {
	if err != nil {
		return nil, err
	}
	ch := make(chan llm.StreamEvent, len(resp.Message.Content)+2)
	go func() {
		defer close(ch)
		send := func(e llm.StreamEvent) bool {
			select {
			case <-ctx.Done():
				return false
			case ch <- e:
				return true
			}
		}
		if !send(llm.MessageStartEvent(resp.ID, resp.Model)) {
			return
		}
		idx := 0
		for _, p := range resp.Message.Content {
			switch p.Kind {
			case llm.KindText:
				if !send(llm.ContentDeltaEvent(p.Text, idx)) {
					return
				}
			case llm.KindThinking, llm.KindRedactedThinking:
				if p.Thinking != nil && !send(llm.ThinkingDeltaEvent(p.Thinking.Text, idx)) {
					return
				}
			case llm.KindToolCall:
				if p.ToolCall != nil {
					if !send(llm.ToolCallStartEvent(p.ToolCall.ID, p.ToolCall.Name, idx)) {
						return
					}
				}
			}
			idx++
		}
		usage := resp.Usage
		send(llm.MessageEndEvent(resp.FinishReason, &usage))
	}()
	return ch, nil
}
