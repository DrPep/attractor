package adapters

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
)

// Fix 7: the last user message gets an ephemeral cache breakpoint that actually
// serializes (the zero-value param is omitted, so the constructor is required).
func TestInjectUserCacheControl(t *testing.T) {
	msgs := []anthropic.MessageParam{
		anthropic.NewAssistantMessage(anthropic.NewTextBlock("earlier")),
		anthropic.NewUserMessage(anthropic.NewTextBlock("latest user turn")),
	}
	injectUserCacheControl(msgs)

	b, err := json.Marshal(msgs[1])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "cache_control") {
		t.Errorf("last user message has no cache_control: %s", b)
	}
	// The earlier assistant message must NOT be cached.
	ab, _ := json.Marshal(msgs[0])
	if strings.Contains(string(ab), "cache_control") {
		t.Errorf("assistant message should not be cached: %s", ab)
	}
}
