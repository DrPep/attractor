// Package aerr defines the Attractor error hierarchy shared across subsystems.
// It mirrors attractor/exceptions.py from the original Python implementation.
package aerr

import "fmt"

// ConfigurationError indicates invalid configuration.
type ConfigurationError struct{ Msg string }

func (e *ConfigurationError) Error() string { return e.Msg }

// TimeoutError indicates an operation timed out.
type TimeoutError struct{ Msg string }

func (e *TimeoutError) Error() string { return e.Msg }

// RateLimitError indicates a provider rate limit was exceeded.
type RateLimitError struct {
	Msg        string
	RetryAfter float64 // seconds; 0 if unknown
}

func (e *RateLimitError) Error() string { return e.Msg }

// AuthenticationError indicates authentication with a provider failed.
type AuthenticationError struct {
	Provider string
	Msg      string
}

func (e *AuthenticationError) Error() string { return e.Msg }

// ProviderError is a generic error from a specific LLM provider.
type ProviderError struct {
	Provider   string
	Msg        string
	StatusCode int // 0 if unknown
}

func (e *ProviderError) Error() string {
	if e.Provider != "" {
		return fmt.Sprintf("%s: %s", e.Provider, e.Msg)
	}
	return e.Msg
}

// ValidationError indicates graph or condition validation failed.
type ValidationError struct{ Msg string }

func (e *ValidationError) Error() string { return e.Msg }

// PipelineError indicates an error during pipeline execution.
type PipelineError struct{ Msg string }

func (e *PipelineError) Error() string { return e.Msg }

// ParseError indicates a DOT file could not be parsed.
type ParseError struct{ Msg string }

func (e *ParseError) Error() string { return e.Msg }

// GoalGateError indicates a goal gate was not satisfied.
type GoalGateError struct{ Msg string }

func (e *GoalGateError) Error() string { return e.Msg }
