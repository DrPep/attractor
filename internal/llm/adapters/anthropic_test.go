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

// Opus 4.7 and later take adaptive thinking plus an effort level. Sending them a
// thinking token budget or a sampling param is a 400.
func TestBuildParamsThinkingAdaptive(t *testing.T) {
	a := &AnthropicAdapter{}
	temp := 0.5
	params, err := a.buildParams(llm.Request{
		Model:           "claude-opus-4-7",
		ReasoningEffort: "high",
		Temperature:     &temp,
		MaxTokens:       4096,
		Messages:        []llm.Message{llm.UserMessage("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if params.Thinking.OfAdaptive == nil {
		t.Errorf("thinking should be adaptive: %+v", params.Thinking)
	}
	if params.Thinking.OfEnabled != nil {
		t.Errorf("thinking must not use a token budget on Opus 4.7 (400): %+v", params.Thinking)
	}
	if params.OutputConfig.Effort != anthropic.OutputConfigEffortHigh {
		t.Errorf("effort = %q, want high", params.OutputConfig.Effort)
	}
	// max_tokens is the caller's, not grown around a budget.
	if params.MaxTokens != 4096 {
		t.Errorf("max_tokens = %d, want 4096 untouched", params.MaxTokens)
	}
	if params.Temperature.Valid() {
		t.Errorf("temperature must be omitted on Opus 4.7 (400)")
	}

	// Sampling params are rejected regardless of whether thinking is on, so an
	// effort-less request must still omit temperature.
	noThink, err := a.buildParams(llm.Request{
		Model:       "claude-opus-4-7",
		Temperature: &temp,
		Messages:    []llm.Message{llm.UserMessage("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if noThink.Thinking.OfAdaptive != nil || noThink.Thinking.OfEnabled != nil {
		t.Errorf("thinking should be off without effort: %+v", noThink.Thinking)
	}
	if noThink.Temperature.Valid() {
		t.Errorf("temperature must be omitted on Opus 4.7 even with thinking off (400)")
	}

	// An uncatalogued claude model must default to the current contract rather
	// than the removed token budget.
	future, err := a.buildParams(llm.Request{
		Model:           "claude-opus-9-9",
		ReasoningEffort: "xhigh",
		Messages:        []llm.Message{llm.UserMessage("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if future.Thinking.OfAdaptive == nil || future.Thinking.OfEnabled != nil {
		t.Errorf("unknown model should default to adaptive: %+v", future.Thinking)
	}
	if future.OutputConfig.Effort != anthropic.OutputConfigEffortXhigh {
		t.Errorf("effort = %q, want xhigh", future.OutputConfig.Effort)
	}
}

// The 4.6 generation takes an effort level but has no "xhigh", so xhigh must be
// downgraded rather than sent through. It still accepts sampling params.
func TestBuildParamsEffortWithoutXHigh(t *testing.T) {
	a := &AnthropicAdapter{}
	params, err := a.buildParams(llm.Request{
		Model:           "claude-opus-4-6",
		ReasoningEffort: "xhigh",
		Messages:        []llm.Message{llm.UserMessage("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if params.OutputConfig.Effort != anthropic.OutputConfigEffortHigh {
		t.Errorf("effort = %q, want high (4.6 has no xhigh)", params.OutputConfig.Effort)
	}

	temp := 0.5
	sampling, err := a.buildParams(llm.Request{
		Model:       "claude-opus-4-6",
		Temperature: &temp,
		Messages:    []llm.Message{llm.UserMessage("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !sampling.Temperature.Valid() {
		t.Errorf("temperature should be forwarded on Opus 4.6 with thinking off")
	}
}

// Sonnet 4.5 and older still take a thinking token budget, which must be smaller
// than max_tokens — so max_tokens grows to leave room for the visible response.
func TestBuildParamsThinkingTokenBudget(t *testing.T) {
	a := &AnthropicAdapter{}
	params, err := a.buildParams(llm.Request{
		Model:           "claude-sonnet-4-5",
		ReasoningEffort: "high",
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
	// Legacy models take no effort field.
	if params.OutputConfig.Effort != "" {
		t.Errorf("effort = %q, want unset on a token-budget model", params.OutputConfig.Effort)
	}
}

// A non-reasoning model (Haiku) must not get a thinking block even with the
// pipeline's default "high" effort, or the API would reject the request.
func TestBuildParamsNonReasoningModel(t *testing.T) {
	a := &AnthropicAdapter{}
	haiku, err := a.buildParams(llm.Request{
		Model:           "claude-haiku-4-5",
		ReasoningEffort: "high",
		Messages:        []llm.Message{llm.UserMessage("hi")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if haiku.Thinking.OfEnabled != nil || haiku.Thinking.OfAdaptive != nil {
		t.Errorf("thinking should be gated off for a non-reasoning model: %+v", haiku.Thinking)
	}
	if haiku.OutputConfig.Effort != "" {
		t.Errorf("effort = %q, want unset", haiku.OutputConfig.Effort)
	}
}
