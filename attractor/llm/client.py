"""Core LLM Client with provider routing and middleware."""

from __future__ import annotations

import os
from typing import Any, AsyncIterator

from .adapter import ProviderAdapter
from .catalog import resolve_provider
from .middleware import Middleware, build_chain
from .models import Request, Response
from .streaming import StreamEvent
from ..exceptions import ConfigurationError


class Client:
    """Unified LLM client with provider routing and middleware."""

    def __init__(
        self,
        providers: dict[str, ProviderAdapter] | None = None,
        default_provider: str | None = None,
        middleware: list[Middleware] | None = None,
    ):
        self._providers: dict[str, ProviderAdapter] = providers or {}
        self._default_provider = default_provider
        self._middleware = middleware or []

        if not self._default_provider and self._providers:
            self._default_provider = next(iter(self._providers))

    @classmethod
    def from_env(cls, middleware: list[Middleware] | None = None) -> Client:
        """Create client from environment variables. Only registers available providers."""
        providers: dict[str, ProviderAdapter] = {}

        if os.environ.get("OPENAI_API_KEY"):
            try:
                from .adapters.openai import OpenAIAdapter
                providers["openai"] = OpenAIAdapter(
                    api_key=os.environ["OPENAI_API_KEY"],
                    base_url=os.environ.get("OPENAI_BASE_URL"),
                    org_id=os.environ.get("OPENAI_ORG_ID"),
                )
            except ImportError:
                pass

        if os.environ.get("ANTHROPIC_API_KEY"):
            try:
                from .adapters.anthropic import AnthropicAdapter
                providers["anthropic"] = AnthropicAdapter(
                    api_key=os.environ["ANTHROPIC_API_KEY"],
                    base_url=os.environ.get("ANTHROPIC_BASE_URL"),
                )
            except ImportError:
                pass

        if os.environ.get("GEMINI_API_KEY"):
            try:
                from .adapters.gemini import GeminiAdapter
                providers["gemini"] = GeminiAdapter(
                    api_key=os.environ["GEMINI_API_KEY"],
                    base_url=os.environ.get("GEMINI_BASE_URL"),
                )
            except ImportError:
                pass

        return cls(providers=providers, middleware=middleware)

    def register_provider(self, name: str, adapter: ProviderAdapter) -> None:
        self._providers[name] = adapter
        if not self._default_provider:
            self._default_provider = name

    def _resolve_provider(self, request: Request) -> ProviderAdapter:
        provider_name = request.provider
        if not provider_name:
            provider_name = resolve_provider(request.model)
        if not provider_name:
            provider_name = self._default_provider
        if not provider_name or provider_name not in self._providers:
            available = list(self._providers.keys())
            raise ConfigurationError(
                f"No provider found for model '{request.model}'. "
                f"Available providers: {available}"
            )
        return self._providers[provider_name]

    async def complete(self, request: Request) -> Response:
        adapter = self._resolve_provider(request)

        async def final(req: Request) -> Response:
            response = await adapter.complete(req)
            response.provider = adapter.provider_name
            return response

        chain = build_chain(self._middleware, final)
        return await chain(request)

    async def stream(self, request: Request) -> AsyncIterator[StreamEvent]:
        adapter = self._resolve_provider(request)
        async for event in adapter.stream(request):
            yield event

    async def close(self) -> None:
        for adapter in self._providers.values():
            await adapter.close()


# Module-level default client
_default_client: Client | None = None


def set_default_client(client: Client) -> None:
    global _default_client
    _default_client = client


def get_default_client() -> Client:
    global _default_client
    if _default_client is None:
        _default_client = Client.from_env()
    return _default_client


async def generate(
    model: str, prompt: str, client: Client | None = None, **kwargs: Any,
) -> Response:
    """High-level generate function using default client."""
    from .models import Message
    c = client or get_default_client()
    request = Request(
        model=model,
        messages=[Message.user(prompt)],
        **kwargs,
    )
    return await c.complete(request)


async def stream(
    model: str, prompt: str, client: Client | None = None, **kwargs: Any,
) -> AsyncIterator[StreamEvent]:
    """High-level stream function using default client."""
    from .models import Message
    c = client or get_default_client()
    request = Request(
        model=model,
        messages=[Message.user(prompt)],
        **kwargs,
    )
    async for event in c.stream(request):
        yield event
