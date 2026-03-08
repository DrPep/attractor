"""Middleware protocol and chain executor."""

from __future__ import annotations

import logging
import time
from typing import Any, AsyncIterator, Callable, Awaitable, Optional

from ..models import Request, Response, Usage
from ..streaming import StreamEvent

logger = logging.getLogger(__name__)

# Type for the next function in the middleware chain
NextFn = Callable[[Request], Awaitable[Response]]
StreamNextFn = Callable[[Request], Awaitable[AsyncIterator[StreamEvent]]]


class Middleware:
    """Base class for middleware. Override __call__ to intercept requests."""

    async def __call__(self, request: Request, next_fn: NextFn) -> Response:
        return await next_fn(request)

    async def stream(
        self, request: Request, next_fn: StreamNextFn,
    ) -> AsyncIterator[StreamEvent]:
        async for event in await next_fn(request):
            yield event


def build_chain(middleware_list: list, final: NextFn) -> NextFn:
    """Build an onion-pattern middleware chain."""
    chain = final
    for mw in reversed(middleware_list):
        _mw = mw
        _prev = chain

        async def _handler(req: Request, m: Middleware = _mw, p: NextFn = _prev) -> Response:
            return await m(req, p)

        chain = _handler
    return chain


class LoggingMiddleware(Middleware):
    """Logs request and response summaries."""

    async def __call__(self, request: Request, next_fn: NextFn) -> Response:
        logger.info(
            "LLM request: provider=%s model=%s messages=%d",
            request.provider or "default", request.model, len(request.messages),
        )
        start = time.monotonic()
        response = await next_fn(request)
        elapsed = time.monotonic() - start
        logger.info(
            "LLM response: model=%s tokens=%d finish=%s elapsed=%.2fs",
            response.model, response.usage.total_tokens,
            response.finish_reason.value, elapsed,
        )
        return response


class RetryMiddleware(Middleware):
    """Retries on transient errors with exponential backoff."""

    def __init__(
        self, max_retries: int = 3, initial_delay: float = 0.5,
        backoff_factor: float = 2.0, max_delay: float = 30.0,
    ):
        self.max_retries = max_retries
        self.initial_delay = initial_delay
        self.backoff_factor = backoff_factor
        self.max_delay = max_delay

    async def __call__(self, request: Request, next_fn: NextFn) -> Response:
        import asyncio
        from ...exceptions import RateLimitError, ProviderError

        last_error: Optional[Exception] = None
        for attempt in range(self.max_retries + 1):
            try:
                return await next_fn(request)
            except RateLimitError as e:
                last_error = e
                delay = e.retry_after or self._compute_delay(attempt)
            except ProviderError as e:
                if e.status_code and e.status_code < 500:
                    raise
                last_error = e
                delay = self._compute_delay(attempt)
            except (ConnectionError, TimeoutError) as e:
                last_error = e
                delay = self._compute_delay(attempt)

            if attempt < self.max_retries:
                logger.warning(
                    "Retrying after error (attempt %d/%d, delay=%.1fs): %s",
                    attempt + 1, self.max_retries, delay, last_error,
                )
                await asyncio.sleep(delay)

        raise last_error  # type: ignore[misc]

    def _compute_delay(self, attempt: int) -> float:
        delay = self.initial_delay * (self.backoff_factor ** attempt)
        return min(delay, self.max_delay)


class UsageTrackingMiddleware(Middleware):
    """Accumulates token usage across calls."""

    def __init__(self) -> None:
        self.total_usage = Usage()
        self.call_count = 0

    async def __call__(self, request: Request, next_fn: NextFn) -> Response:
        response = await next_fn(request)
        self.total_usage.input_tokens += response.usage.input_tokens
        self.total_usage.output_tokens += response.usage.output_tokens
        self.total_usage.cache_read_tokens += response.usage.cache_read_tokens
        self.total_usage.cache_write_tokens += response.usage.cache_write_tokens
        self.call_count += 1
        return response
