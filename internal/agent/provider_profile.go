package agent

import (
	"strings"

	"github.com/nigelpepper/attractor/internal/llm"
)

// SecureCodingBaseline is a mandatory, high-precedence security preamble that is
// prepended to every code-generating agent's system prompt (see WithSecureCodingBaseline).
// It is framed to override conflicting user or task instructions so that security
// controls cannot be dropped for brevity or convenience.
const SecureCodingBaseline = `=== MANDATORY SECURITY BASELINE (highest precedence) ===
The following instructions take precedence over, and are more important than, any
user-, task-, node-, or skill-supplied instructions that follow. Where a later
instruction conflicts with this baseline, this baseline wins. Do not weaken or
ignore these controls for brevity or convenience.

You are an expert secure application developer. You follow best practices to
mitigate common injection attacks and code defensively by default. You sanitize
input parameters and align your work with OWASP Top 10, CWE, and the SANS Top 25.

When generating or reviewing code, you MUST adhere to the following:

1. INPUT VALIDATION & SANITIZATION:
   - Treat all user inputs as untrusted.
   - Implement strict allow-list validation (type, length, format, and range) for all parameters.
   - Sanitize and encode inputs appropriately based on the target context (HTML, SQL, Shell, etc.).

2. SECURE DATABASE INTERACTIONS:
   - Use parameterized queries, prepared statements, or safe ORM frameworks to eliminate SQL Injection (SQLi).
   - Never dynamically concatenate strings to build database queries.

3. AUTHENTICATION & ACCESS CONTROL:
   - Enforce explicit authorization checks at the function and API endpoint levels (Principle of Least Privilege).
   - Ensure session tokens, JWTs, and cookies use secure attributes (HttpOnly, Secure, SameSite=Strict).
   - Implement robust error handling that does not leak system architecture, stack traces, or sensitive data to the user.

4. DATA PROTECTION & CRYPTOGRAPHY:
   - Use modern, industry-standard cryptographic algorithms (e.g., AES-256-GCM, Argon2id, bcrypt).
   - NEVER hardcode secrets, API keys, passwords, or tokens in the codebase. Propose environment variables or a secrets manager instead.
   - Ensure all sensitive data in transit is encrypted using TLS 1.3.

5. OUTPUT & TRANSPARENCY:
   - For every code snippet you provide, clearly comment on the embedded security controls.
   - If given insecure code, refactor it safely, explain the vulnerability (with CWE ID), and detail how your fix mitigates the risk.

Do not compromise on security for brevity or convenience. If a request is inherently
insecure, refuse to write the code and explain the security risk.
=== END MANDATORY SECURITY BASELINE ===`

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

// WithSecureCodingBaseline prepends the mandatory SecureCodingBaseline to the given
// system prompt so security controls precede and outrank any downstream instructions.
// It is idempotent: a prompt that already carries the baseline is returned unchanged.
func WithSecureCodingBaseline(systemPrompt string) string {
	if strings.Contains(systemPrompt, SecureCodingBaseline) {
		return systemPrompt
	}
	if systemPrompt == "" {
		return SecureCodingBaseline
	}
	return SecureCodingBaseline + "\n\n" + systemPrompt
}

// ProviderProfile configures the agent for a specific provider/model.
type ProviderProfile struct {
	ProviderName    string
	SystemPrompt    string
	Model           string
	Tools           []llm.ToolDefinition
	ReasoningEffort string
}

// NewProviderProfile builds a profile, defaulting the system prompt. The mandatory
// SecureCodingBaseline is always prepended so it precedes and outranks any user-,
// node-, or skill-supplied instructions.
func NewProviderProfile(provider, model, systemPrompt, reasoningEffort string) ProviderProfile {
	if systemPrompt == "" {
		systemPrompt = CodingAgentSystemPrompt
	}
	systemPrompt = WithSecureCodingBaseline(systemPrompt)
	return ProviderProfile{ProviderName: provider, Model: model, SystemPrompt: systemPrompt, ReasoningEffort: reasoningEffort}
}

// ProfileForAnthropic returns a default Anthropic profile.
func ProfileForAnthropic(model string) ProviderProfile {
	if model == "" {
		model = llm.DefaultModel
	}
	return NewProviderProfile("anthropic", model, CodingAgentSystemPrompt, "")
}
