"""Retry mechanism with backoff policies."""

from __future__ import annotations

import random

from pydantic import BaseModel

from ..exceptions import RateLimitError, ProviderError, AuthenticationError


class RetryPolicy(BaseModel):
    max_retries: int = 0
    initial_delay_ms: float = 200
    backoff_factor: float = 2.0
    max_delay_ms: float = 30000
    jitter: bool = True


# Preset policies
RETRY_NONE = RetryPolicy(max_retries=0)
RETRY_STANDARD = RetryPolicy(
    max_retries=4, initial_delay_ms=200, backoff_factor=2.0,
    max_delay_ms=30000, jitter=True,
)
RETRY_AGGRESSIVE = RetryPolicy(
    max_retries=4, initial_delay_ms=500, backoff_factor=2.0,
    max_delay_ms=60000, jitter=True,
)
RETRY_LINEAR = RetryPolicy(
    max_retries=2, initial_delay_ms=500, backoff_factor=1.0,
    max_delay_ms=500, jitter=False,
)
RETRY_PATIENT = RetryPolicy(
    max_retries=2, initial_delay_ms=2000, backoff_factor=2.0,
    max_delay_ms=30000, jitter=True,
)

PRESETS: dict[str, RetryPolicy] = {
    "none": RETRY_NONE,
    "standard": RETRY_STANDARD,
    "aggressive": RETRY_AGGRESSIVE,
    "linear": RETRY_LINEAR,
    "patient": RETRY_PATIENT,
}


def compute_delay(attempt: int, policy: RetryPolicy) -> float:
    """Compute delay in seconds for a retry attempt."""
    delay_ms = policy.initial_delay_ms * (policy.backoff_factor ** attempt)
    delay_ms = min(delay_ms, policy.max_delay_ms)
    if policy.jitter:
        delay_ms *= random.uniform(0.5, 1.5)
    return delay_ms / 1000.0


def is_retryable(error: Exception) -> bool:
    """Check if an error should trigger a retry."""
    if isinstance(error, RateLimitError):
        return True
    if isinstance(error, AuthenticationError):
        return False
    if isinstance(error, ProviderError):
        if error.status_code:
            # 4xx except 429 are not retryable
            if 400 <= error.status_code < 500 and error.status_code != 429:
                return False
            # 5xx are retryable
            if error.status_code >= 500:
                return True
        return False
    if isinstance(error, (ConnectionError, TimeoutError, OSError)):
        return True
    return False
