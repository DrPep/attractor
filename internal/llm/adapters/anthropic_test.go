package adapters

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/nigelpepper/attractor/internal/llm"
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

func TestAnthropicThinkingBudget(t *testing.T) {
	cases := map[string]int64{
		"": 0, "none": 0, "low": 2048, "medium": 8192,
		"high": 16384, "xhigh": 32000, "weird": 8192,
	}
	for effort, want := range cases {
		if got := anthropicThinkingBudget(effort); got != want {
			t.Errorf("budget(%q) = %d, want %d", effort, got, want)
		}
	}
}

// Enabling thinking must set the thinking block, drop temperature (Anthropic
// rejects a custom temperature with thinking on), and grow max_tokens above the
// thinking budget.
func TestBuildParamsThinking(t *testing.T) {
	a := &AnthropicAdapter{}
	temp := 0.5
	params, err := a.buildParams(llm.Request{
		Model:           "claude-opus-4-7",
		ReasoningEffort: "high",
		Temperature:     &temp,
		MaxTokens:       4096, // below the 16384 budget → must be grown
		Messages:        []llm.Message{llm.UserMessage("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if params.Thinking.OfEnabled == nil || params.Thinking.OfEnabled.BudgetTokens != 16384 {
		t.Errorf("thinking not enabled with budget 16384: %+v", params.Thinking)
	}
	if params.MaxTokens <= 16384 {
		t.Errorf("max_tokens = %d, want > 16384 budget", params.MaxTokens)
	}
	if params.Temperature.Valid() {
		t.Errorf("temperature should be omitted when thinking is on")
	}

	// A non-reasoning model (Haiku) must not get a thinking block even with a
	// high effort default, or the API would reject the request.
	haiku, err := a.buildParams(llm.Request{
		Model:           "claude-haiku-4-5",
		ReasoningEffort: "high",
		Messages:        []llm.Message{llm.UserMessage("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if haiku.Thinking.OfEnabled != nil {
		t.Errorf("thinking should be gated off for a non-reasoning model")
	}

	// Without effort, temperature is forwarded and thinking stays off.
	noThink, err := a.buildParams(llm.Request{
		Model:       "claude-opus-4-7",
		Temperature: &temp,
		Messages:    []llm.Message{llm.UserMessage("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if noThink.Thinking.OfEnabled != nil {
		t.Errorf("thinking should be off without effort")
	}
	if !noThink.Temperature.Valid() {
		t.Errorf("temperature should be forwarded when thinking is off")
	}
}
