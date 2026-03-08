"""Base exception hierarchy for Attractor."""

from __future__ import annotations


class AttractorError(Exception):
    """Base exception for all Attractor errors."""


class LLMError(AttractorError):
    """Error from LLM interaction."""


class ProviderError(LLMError):
    """Error from a specific LLM provider."""

    def __init__(self, message: str, provider: str, status_code: int | None = None):
        super().__init__(message)
        self.provider = provider
        self.status_code = status_code


class ConfigurationError(AttractorError):
    """Invalid configuration."""


class AttractorTimeoutError(AttractorError):
    """Operation timed out."""


class RateLimitError(LLMError):
    """Rate limit exceeded."""

    def __init__(self, message: str, retry_after: float | None = None):
        super().__init__(message)
        self.retry_after = retry_after


class AuthenticationError(LLMError):
    """Authentication failed."""


class AttractorValidationError(AttractorError):
    """Validation failed."""


class PipelineError(AttractorError):
    """Error in pipeline execution."""


class ParseError(PipelineError):
    """Error parsing DOT file."""


class GoalGateError(PipelineError):
    """Goal gate not satisfied."""
