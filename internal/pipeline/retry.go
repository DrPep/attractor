package pipeline

import (
	"errors"
	"math"
	"math/rand"
	"time"

	"github.com/nigelpepper/attractor/internal/aerr"
)

// RetryPolicy parameterizes backoff behavior.
type RetryPolicy struct {
	MaxRetries     int
	InitialDelayMS float64
	BackoffFactor  float64
	MaxDelayMS     float64
	Jitter         bool
}

// Preset retry policies.
var (
	RetryNone       = RetryPolicy{MaxRetries: 0}
	RetryStandard   = RetryPolicy{MaxRetries: 4, InitialDelayMS: 200, BackoffFactor: 2.0, MaxDelayMS: 30000, Jitter: true}
	RetryAggressive = RetryPolicy{MaxRetries: 4, InitialDelayMS: 500, BackoffFactor: 2.0, MaxDelayMS: 60000, Jitter: true}
	RetryLinear     = RetryPolicy{MaxRetries: 2, InitialDelayMS: 500, BackoffFactor: 1.0, MaxDelayMS: 500, Jitter: false}
	RetryPatient    = RetryPolicy{MaxRetries: 2, InitialDelayMS: 2000, BackoffFactor: 2.0, MaxDelayMS: 30000, Jitter: true}
)

// Presets maps preset names to policies.
var Presets = map[string]RetryPolicy{
	"none":       RetryNone,
	"standard":   RetryStandard,
	"aggressive": RetryAggressive,
	"linear":     RetryLinear,
	"patient":    RetryPatient,
}

// ComputeDelay returns the backoff delay for a retry attempt.
func ComputeDelay(attempt int, policy RetryPolicy) time.Duration {
	delayMS := policy.InitialDelayMS * math.Pow(policy.BackoffFactor, float64(attempt))
	if delayMS > policy.MaxDelayMS {
		delayMS = policy.MaxDelayMS
	}
	if policy.Jitter {
		delayMS *= 0.5 + rand.Float64()
	}
	return time.Duration(delayMS) * time.Millisecond
}

// IsRetryable reports whether an error should trigger a retry.
func IsRetryable(err error) bool {
	var rl *aerr.RateLimitError
	if errors.As(err, &rl) {
		return true
	}
	var auth *aerr.AuthenticationError
	if errors.As(err, &auth) {
		return false
	}
	var pe *aerr.ProviderError
	if errors.As(err, &pe) {
		if pe.StatusCode != 0 {
			if pe.StatusCode >= 400 && pe.StatusCode < 500 && pe.StatusCode != 429 {
				return false
			}
			if pe.StatusCode >= 500 {
				return true
			}
		}
		return false
	}
	var to *aerr.TimeoutError
	return errors.As(err, &to)
}
