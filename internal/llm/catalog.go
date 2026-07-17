package llm

import "strings"

// DefaultModel is the model used when a node, profile, or session config does
// not name one.
const DefaultModel = "claude-opus-4-8"

// ReasoningStyle describes how a model accepts reasoning configuration.
type ReasoningStyle string

const (
	// ReasoningNone means the model has no reasoning mode.
	ReasoningNone ReasoningStyle = ""
	// ReasoningTokenBudget configures reasoning with an explicit token budget
	// (Anthropic's thinking.type=enabled plus budget_tokens). Anthropic removed
	// this after the 4.6 generation: newer models reject it with a 400.
	ReasoningTokenBudget ReasoningStyle = "token_budget"
	// ReasoningEffort configures reasoning with a named effort level rather than
	// a token count (Anthropic's thinking.type=adaptive plus output_config.effort,
	// OpenAI's reasoning.effort).
	ReasoningEffort ReasoningStyle = "effort"
)

// ModelInfo describes a known model and its capabilities.
type ModelInfo struct {
	ID             string
	Provider       string
	DisplayName    string
	ContextWindow  int
	MaxOutput      int
	SupportsTools  bool
	SupportsVision bool
	// Reasoning is how the model accepts reasoning configuration, if at all.
	Reasoning ReasoningStyle
	// SupportsSampling reports whether the model accepts temperature/top_p/top_k.
	// Anthropic removed them from Opus 4.7 and later, which reject them with a 400.
	SupportsSampling bool
	// SupportsXHighEffort reports whether the model accepts the "xhigh" effort
	// level, which arrived with Opus 4.7. Models without it take "high" instead.
	SupportsXHighEffort  bool
	InputCostPerMillion  float64
	OutputCostPerMillion float64
	Aliases              []string
}

// Models is the catalog of known models.
var Models = []ModelInfo{
	// Anthropic. Opus 4.7 and later take adaptive thinking plus an effort level
	// and reject sampling params; the 4.6 generation takes effort but has no
	// "xhigh" level; older models still take a thinking token budget.
	{ID: "claude-fable-5", Provider: "anthropic", DisplayName: "Claude Fable 5", ContextWindow: 1000000, MaxOutput: 128000, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsXHighEffort: true, InputCostPerMillion: 10.0, OutputCostPerMillion: 50.0, Aliases: []string{"fable"}},
	{ID: "claude-opus-4-8", Provider: "anthropic", DisplayName: "Claude Opus 4.8", ContextWindow: 1000000, MaxOutput: 128000, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsXHighEffort: true, InputCostPerMillion: 5.0, OutputCostPerMillion: 25.0, Aliases: []string{"opus"}},
	{ID: "claude-opus-4-7", Provider: "anthropic", DisplayName: "Claude Opus 4.7", ContextWindow: 1000000, MaxOutput: 128000, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsXHighEffort: true, InputCostPerMillion: 5.0, OutputCostPerMillion: 25.0},
	{ID: "claude-opus-4-6", Provider: "anthropic", DisplayName: "Claude Opus 4.6", ContextWindow: 1000000, MaxOutput: 128000, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsSampling: true, InputCostPerMillion: 5.0, OutputCostPerMillion: 25.0},
	{ID: "claude-sonnet-5", Provider: "anthropic", DisplayName: "Claude Sonnet 5", ContextWindow: 1000000, MaxOutput: 128000, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsXHighEffort: true, InputCostPerMillion: 3.0, OutputCostPerMillion: 15.0, Aliases: []string{"sonnet"}},
	{ID: "claude-sonnet-4-6", Provider: "anthropic", DisplayName: "Claude Sonnet 4.6", ContextWindow: 1000000, MaxOutput: 128000, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsSampling: true, InputCostPerMillion: 3.0, OutputCostPerMillion: 15.0},
	{ID: "claude-sonnet-4-5", Provider: "anthropic", DisplayName: "Claude Sonnet 4.5", ContextWindow: 200000, MaxOutput: 16000, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningTokenBudget, SupportsSampling: true, InputCostPerMillion: 3.0, OutputCostPerMillion: 15.0},
	{ID: "claude-haiku-4-5", Provider: "anthropic", DisplayName: "Claude Haiku 4.5", ContextWindow: 200000, MaxOutput: 64000, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningNone, SupportsSampling: true, InputCostPerMillion: 1.0, OutputCostPerMillion: 5.0, Aliases: []string{"haiku"}},
	// OpenAI
	{ID: "gpt-5.2", Provider: "openai", DisplayName: "GPT-5.2", ContextWindow: 1047576, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsSampling: true, Aliases: []string{"gpt5"}},
	{ID: "gpt-5.2-mini", Provider: "openai", DisplayName: "GPT-5.2 Mini", ContextWindow: 1047576, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsSampling: true, Aliases: []string{"gpt5-mini"}},
	{ID: "gpt-5.2-codex", Provider: "openai", DisplayName: "GPT-5.2 Codex", ContextWindow: 1047576, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsSampling: true, Aliases: []string{"codex"}},
	{ID: "gpt-4.1", Provider: "openai", DisplayName: "GPT-4.1", ContextWindow: 1047576, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningNone, SupportsSampling: true, Aliases: []string{"gpt4.1"}},
	// Gemini
	{ID: "gemini-3-pro-preview", Provider: "gemini", DisplayName: "Gemini 3 Pro (Preview)", ContextWindow: 1048576, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsSampling: true, Aliases: []string{"gemini-pro"}},
	{ID: "gemini-3-flash-preview", Provider: "gemini", DisplayName: "Gemini 3 Flash (Preview)", ContextWindow: 1048576, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsSampling: true, Aliases: []string{"gemini-flash"}},
	{ID: "gemini-2.5-flash", Provider: "gemini", DisplayName: "Gemini 2.5 Flash", ContextWindow: 1048576, SupportsTools: true, SupportsVision: true, Reasoning: ReasoningEffort, SupportsSampling: true, Aliases: []string{"gemini-2.5"}},
}

var (
	modelIndex = map[string]ModelInfo{}
	aliasIndex = map[string]ModelInfo{}
)

func init() {
	for _, m := range Models {
		modelIndex[m.ID] = m
		for _, a := range m.Aliases {
			aliasIndex[a] = m
		}
	}
}

// GetModelInfo returns model metadata by id or alias, and whether it was found.
func GetModelInfo(modelID string) (ModelInfo, bool) {
	if m, ok := modelIndex[modelID]; ok {
		return m, true
	}
	m, ok := aliasIndex[modelID]
	return m, ok
}

// ListModels returns all models, optionally filtered by provider.
func ListModels(provider string) []ModelInfo {
	if provider == "" {
		out := make([]ModelInfo, len(Models))
		copy(out, Models)
		return out
	}
	var out []ModelInfo
	for _, m := range Models {
		if m.Provider == provider {
			out = append(out, m)
		}
	}
	return out
}

// ResolveProvider returns the provider name for a model id/alias. It first
// consults the catalog (exact ids and aliases), then falls back to inferring
// the provider from the model id's naming convention. The inference step lets
// newly released models route correctly without a catalog entry — the catalog
// is for metadata and overrides, not a gate on which models can be used.
func ResolveProvider(modelID string) string {
	if m, ok := GetModelInfo(modelID); ok {
		return m.Provider
	}
	return inferProvider(modelID)
}

// inferProvider guesses the provider from a model id's naming convention.
// Vendors keep stable id prefixes across releases, so this resolves new models
// (e.g. a future "claude-*" or "gpt-*") without code changes. Returns "" when
// the id matches no known convention; callers then fall back to the default
// provider or an explicit Request.Provider override.
func inferProvider(modelID string) string {
	id := strings.ToLower(strings.TrimSpace(modelID))
	switch {
	case strings.HasPrefix(id, "claude"):
		return "anthropic"
	case strings.HasPrefix(id, "gpt"), strings.HasPrefix(id, "chatgpt"), isOpenAIReasoningID(id):
		return "openai"
	case strings.HasPrefix(id, "gemini"):
		return "gemini"
	}
	return ""
}

// isOpenAIReasoningID reports whether id matches OpenAI's reasoning-model
// naming (o1, o3, o4, o5, ...): a leading "o" followed by a digit.
func isOpenAIReasoningID(id string) bool {
	return len(id) >= 2 && id[0] == 'o' && id[1] >= '0' && id[1] <= '9'
}
