"""Provider profiles - per-provider tool definitions and system prompts."""

from __future__ import annotations

from pydantic import BaseModel, Field

from ..llm.models import ToolDefinition


CODING_AGENT_SYSTEM_PROMPT = """\
You are a coding agent. You help users with software engineering tasks by reading files, \
editing code, and running commands. You have access to tools for file operations and shell execution.

Guidelines:
- Read files before modifying them to understand existing code.
- Make minimal, focused changes.
- Prefer editing existing files over creating new ones.
- Run tests after making changes when possible.
- Write secure, correct code.
- Explain what you're doing briefly.
"""


class ProviderProfile(BaseModel):
    """Configuration for a specific LLM provider."""

    provider_name: str
    system_prompt: str = CODING_AGENT_SYSTEM_PROMPT
    model: str
    tools: list[ToolDefinition] = Field(default_factory=list)
    reasoning_effort: str | None = None

    @classmethod
    def for_anthropic(
        cls,
        model: str = "claude-sonnet-4-6",
        tools: list[ToolDefinition] | None = None,
    ) -> ProviderProfile:
        return cls(
            provider_name="anthropic",
            model=model,
            tools=tools or [],
            system_prompt=CODING_AGENT_SYSTEM_PROMPT,
        )

    @classmethod
    def for_openai(
        cls,
        model: str = "gpt-4.1",
        tools: list[ToolDefinition] | None = None,
    ) -> ProviderProfile:
        return cls(
            provider_name="openai",
            model=model,
            tools=tools or [],
            system_prompt=CODING_AGENT_SYSTEM_PROMPT,
        )

    @classmethod
    def for_gemini(
        cls,
        model: str = "gemini-2.5-flash",
        tools: list[ToolDefinition] | None = None,
    ) -> ProviderProfile:
        return cls(
            provider_name="gemini",
            model=model,
            tools=tools or [],
            system_prompt=CODING_AGENT_SYSTEM_PROMPT,
        )
