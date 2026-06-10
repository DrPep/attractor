package adapters

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/anthropics/anthropic-sdk-go"
	openai "github.com/openai/openai-go/v3"

	"github.com/nigelpepper/attractor/internal/aerr"
)

// genCallID returns a synthetic tool-call id for providers (Gemini) that do not
// supply one, mirroring the Python adapters' "call_<hex>" scheme.
func genCallID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return "call_" + hex.EncodeToString(b)
}

// noMessagesError is returned when a request has no translatable messages.
type noMessagesError struct{}

func (e *noMessagesError) Error() string {
	return "cannot send request: no messages after system extraction"
}

func base64Encode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }

// contentToString renders a tool-result content value (string or object) as text.
func contentToString(v any) string {
	switch c := v.(type) {
	case string:
		return c
	case nil:
		return ""
	default:
		b, err := json.Marshal(c)
		if err != nil {
			return ""
		}
		return string(b)
	}
}

// classifyAnthropicError maps an SDK error onto the Attractor error hierarchy so
// the runner's retry classifier (is_retryable) can act on status codes.
func classifyAnthropicError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *anthropic.Error
	if errors.As(err, &apiErr) {
		switch {
		case apiErr.StatusCode == 429:
			return &aerr.RateLimitError{Msg: apiErr.Error()}
		case apiErr.StatusCode == 401 || apiErr.StatusCode == 403:
			return &aerr.AuthenticationError{Provider: "anthropic", Msg: apiErr.Error()}
		default:
			return &aerr.ProviderError{Provider: "anthropic", Msg: apiErr.Error(), StatusCode: apiErr.StatusCode}
		}
	}
	return &aerr.ProviderError{Provider: "anthropic", Msg: err.Error()}
}

// classifyOpenAIError maps an OpenAI SDK error onto the Attractor hierarchy.
func classifyOpenAIError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		return classifyByStatus("openai", apiErr.StatusCode, apiErr.Error())
	}
	return &aerr.ProviderError{Provider: "openai", Msg: err.Error()}
}

// classifyByStatus turns an HTTP status code into the appropriate error type.
func classifyByStatus(provider string, status int, msg string) error {
	switch {
	case status == 429:
		return &aerr.RateLimitError{Msg: msg}
	case status == 401 || status == 403:
		return &aerr.AuthenticationError{Provider: provider, Msg: msg}
	default:
		return &aerr.ProviderError{Provider: provider, Msg: msg, StatusCode: status}
	}
}
