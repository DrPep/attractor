package llm

// ModelInfo describes a known model and its capabilities.
type ModelInfo struct {
	ID                   string
	Provider             string
	DisplayName          string
	ContextWindow        int
	MaxOutput            int
	SupportsTools        bool
	SupportsVision       bool
	SupportsReasoning    bool
	InputCostPerMillion  float64
	OutputCostPerMillion float64
	Aliases              []string
}

// Models is the catalog of known models.
var Models = []ModelInfo{
	// Anthropic
	{ID: "claude-opus-4-7", Provider: "anthropic", DisplayName: "Claude Opus 4.7", ContextWindow: 200000, MaxOutput: 32000, SupportsTools: true, SupportsVision: true, SupportsReasoning: true, InputCostPerMillion: 15.0, OutputCostPerMillion: 75.0, Aliases: []string{"opus"}},
	{ID: "claude-opus-4-6", Provider: "anthropic", DisplayName: "Claude Opus 4.6", ContextWindow: 200000, MaxOutput: 32000, SupportsTools: true, SupportsVision: true, SupportsReasoning: true, InputCostPerMillion: 15.0, OutputCostPerMillion: 75.0},
	{ID: "claude-sonnet-4-6", Provider: "anthropic", DisplayName: "Claude Sonnet 4.6", ContextWindow: 200000, MaxOutput: 16000, SupportsTools: true, SupportsVision: true, SupportsReasoning: true, InputCostPerMillion: 3.0, OutputCostPerMillion: 15.0, Aliases: []string{"sonnet"}},
	{ID: "claude-sonnet-4-5", Provider: "anthropic", DisplayName: "Claude Sonnet 4.5", ContextWindow: 200000, MaxOutput: 16000, SupportsTools: true, SupportsVision: true, SupportsReasoning: true, InputCostPerMillion: 3.0, OutputCostPerMillion: 15.0},
	{ID: "claude-haiku-4-5", Provider: "anthropic", DisplayName: "Claude Haiku 4.5", ContextWindow: 200000, MaxOutput: 8192, SupportsTools: true, SupportsVision: true, SupportsReasoning: false, InputCostPerMillion: 0.80, OutputCostPerMillion: 4.0, Aliases: []string{"haiku"}},
	// OpenAI
	{ID: "gpt-5.2", Provider: "openai", DisplayName: "GPT-5.2", ContextWindow: 1047576, SupportsTools: true, SupportsVision: true, SupportsReasoning: true, Aliases: []string{"gpt5"}},
	{ID: "gpt-5.2-mini", Provider: "openai", DisplayName: "GPT-5.2 Mini", ContextWindow: 1047576, SupportsTools: true, SupportsVision: true, SupportsReasoning: true, Aliases: []string{"gpt5-mini"}},
	{ID: "gpt-5.2-codex", Provider: "openai", DisplayName: "GPT-5.2 Codex", ContextWindow: 1047576, SupportsTools: true, SupportsVision: true, SupportsReasoning: true, Aliases: []string{"codex"}},
	{ID: "gpt-4.1", Provider: "openai", DisplayName: "GPT-4.1", ContextWindow: 1047576, SupportsTools: true, SupportsVision: true, SupportsReasoning: false, Aliases: []string{"gpt4.1"}},
	// Gemini
	{ID: "gemini-3-pro-preview", Provider: "gemini", DisplayName: "Gemini 3 Pro (Preview)", ContextWindow: 1048576, SupportsTools: true, SupportsVision: true, SupportsReasoning: true, Aliases: []string{"gemini-pro"}},
	{ID: "gemini-3-flash-preview", Provider: "gemini", DisplayName: "Gemini 3 Flash (Preview)", ContextWindow: 1048576, SupportsTools: true, SupportsVision: true, SupportsReasoning: true, Aliases: []string{"gemini-flash"}},
	{ID: "gemini-2.5-flash", Provider: "gemini", DisplayName: "Gemini 2.5 Flash", ContextWindow: 1048576, SupportsTools: true, SupportsVision: true, SupportsReasoning: true, Aliases: []string{"gemini-2.5"}},
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

// ResolveProvider returns the provider name for a model id/alias, or "".
func ResolveProvider(modelID string) string {
	if m, ok := GetModelInfo(modelID); ok {
		return m.Provider
	}
	return ""
}
