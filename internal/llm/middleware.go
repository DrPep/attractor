package llm

import (
	"context"
	"log"
	"time"
)

// NextFn is the continuation in a middleware chain.
type NextFn func(ctx context.Context, req Request) (Response, error)

// Middleware intercepts a request on its way to the provider.
type Middleware func(ctx context.Context, req Request, next NextFn) (Response, error)

// buildChain composes middleware in onion order around the final handler.
func buildChain(mws []Middleware, final NextFn) NextFn {
	chain := final
	for i := len(mws) - 1; i >= 0; i-- {
		mw := mws[i]
		prev := chain
		chain = func(ctx context.Context, req Request) (Response, error) {
			return mw(ctx, req, prev)
		}
	}
	return chain
}

// LoggingMiddleware logs request and response summaries.
func LoggingMiddleware() Middleware {
	return func(ctx context.Context, req Request, next NextFn) (Response, error) {
		provider := req.Provider
		if provider == "" {
			provider = "default"
		}
		log.Printf("LLM request: provider=%s model=%s messages=%d", provider, req.Model, len(req.Messages))
		start := time.Now()
		resp, err := next(ctx, req)
		if err != nil {
			return resp, err
		}
		log.Printf("LLM response: model=%s tokens=%d finish=%s elapsed=%.2fs",
			resp.Model, resp.Usage.TotalTokens(), resp.FinishReason, time.Since(start).Seconds())
		return resp, nil
	}
}

// UsageTrackingMiddleware accumulates token usage across calls.
type UsageTrackingMiddleware struct {
	Total     Usage
	CallCount int
}

// Middleware returns the closure to install in a chain.
func (u *UsageTrackingMiddleware) Middleware() Middleware {
	return func(ctx context.Context, req Request, next NextFn) (Response, error) {
		resp, err := next(ctx, req)
		if err != nil {
			return resp, err
		}
		u.Total.InputTokens += resp.Usage.InputTokens
		u.Total.OutputTokens += resp.Usage.OutputTokens
		u.Total.CacheReadTokens += resp.Usage.CacheReadTokens
		u.Total.CacheWriteTokens += resp.Usage.CacheWriteTokens
		u.CallCount++
		return resp, nil
	}
}
