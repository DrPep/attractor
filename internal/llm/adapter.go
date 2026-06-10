package llm

import "context"

// ProviderAdapter is the interface every LLM provider adapter must implement.
type ProviderAdapter interface {
	// ProviderName returns the canonical provider identifier.
	ProviderName() string
	// Complete performs a single non-streaming completion.
	Complete(ctx context.Context, req Request) (Response, error)
	// Stream performs a streaming completion, sending events on the returned
	// channel which is closed when the stream ends.
	Stream(ctx context.Context, req Request) (<-chan StreamEvent, error)
	// Close releases any resources held by the adapter.
	Close() error
}
