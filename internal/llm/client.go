package llm

import (
	"context"
	"fmt"
	"sort"

	"github.com/nigelpepper/attractor/internal/aerr"
)

// Client routes provider-agnostic requests to registered provider adapters,
// applying a middleware chain.
type Client struct {
	providers       map[string]ProviderAdapter
	defaultProvider string
	middleware      []Middleware
}

// NewClient builds a client from a set of adapters and optional middleware.
func NewClient(providers map[string]ProviderAdapter, middleware ...Middleware) *Client {
	c := &Client{providers: map[string]ProviderAdapter{}, middleware: middleware}
	for name, a := range providers {
		c.providers[name] = a
		if c.defaultProvider == "" {
			c.defaultProvider = name
		}
	}
	return c
}

// RegisterProvider adds an adapter under the given name.
func (c *Client) RegisterProvider(name string, adapter ProviderAdapter) {
	if c.providers == nil {
		c.providers = map[string]ProviderAdapter{}
	}
	c.providers[name] = adapter
	if c.defaultProvider == "" {
		c.defaultProvider = name
	}
}

// HasProviders reports whether any provider is registered.
func (c *Client) HasProviders() bool { return len(c.providers) > 0 }

// ProviderNames returns the registered provider names, sorted.
func (c *Client) ProviderNames() []string {
	names := make([]string, 0, len(c.providers))
	for n := range c.providers {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (c *Client) resolveProvider(req Request) (ProviderAdapter, error) {
	name := req.Provider
	if name == "" {
		name = ResolveProvider(req.Model)
	}
	if name == "" {
		name = c.defaultProvider
	}
	adapter, ok := c.providers[name]
	if name == "" || !ok {
		return nil, &aerr.ConfigurationError{Msg: fmt.Sprintf(
			"No provider found for model %q. Available providers: %v", req.Model, c.ProviderNames())}
	}
	return adapter, nil
}

// Complete runs a completion through the middleware chain.
func (c *Client) Complete(ctx context.Context, req Request) (Response, error) {
	adapter, err := c.resolveProvider(req)
	if err != nil {
		return Response{}, err
	}
	final := func(ctx context.Context, req Request) (Response, error) {
		resp, err := adapter.Complete(ctx, req)
		if err != nil {
			return resp, err
		}
		resp.Provider = adapter.ProviderName()
		return resp, nil
	}
	return buildChain(c.middleware, final)(ctx, req)
}

// Stream runs a streaming completion via the resolved adapter.
func (c *Client) Stream(ctx context.Context, req Request) (<-chan StreamEvent, error) {
	adapter, err := c.resolveProvider(req)
	if err != nil {
		return nil, err
	}
	return adapter.Stream(ctx, req)
}

// Close releases all adapter resources.
func (c *Client) Close() error {
	var firstErr error
	for _, a := range c.providers {
		if err := a.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
