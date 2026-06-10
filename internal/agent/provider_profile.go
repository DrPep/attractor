package agent

import "github.com/nigelpepper/attractor/internal/llm"

// CodingAgentSystemPrompt is the default system prompt for the coding agent.
const CodingAgentSystemPrompt = `You are a coding agent. You help users with software engineering tasks by reading files, editing code, and running commands. You have access to tools for file operations and shell execution.

Guidelines:
- Read files before modifying them to understand existing code.
- Make minimal, focused changes.
- Prefer editing existing files over creating new ones.
- Run tests after making changes when possible.
- Write secure, correct code.
- Explain what you're doing briefly.
`

// ProviderProfile configures the agent for a specific provider/model.
type ProviderProfile struct {
	ProviderName    string
	SystemPrompt    string
	Model           string
	Tools           []llm.ToolDefinition
	ReasoningEffort string
}

// NewProviderProfile builds a profile, defaulting the system prompt.
func NewProviderProfile(provider, model, systemPrompt, reasoningEffort string) ProviderProfile {
	if systemPrompt == "" {
		systemPrompt = CodingAgentSystemPrompt
	}
	return ProviderProfile{ProviderName: provider, Model: model, SystemPrompt: systemPrompt, ReasoningEffort: reasoningEffort}
}

// ProfileForAnthropic returns a default Anthropic profile.
func ProfileForAnthropic(model string) ProviderProfile {
	if model == "" {
		model = "claude-opus-4-7"
	}
	return NewProviderProfile("anthropic", model, CodingAgentSystemPrompt, "")
}
