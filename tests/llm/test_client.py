"""Tests for LLM client."""

import pytest
from unittest.mock import AsyncMock, MagicMock

from attractor.llm.client import Client
from attractor.llm.adapter import ProviderAdapter
from attractor.llm.models import (
    ContentPart, FinishReason, Message, Request, Response, Role, Usage,
)
from attractor.llm.middleware import Middleware
from attractor.exceptions import ConfigurationError


class MockAdapter(ProviderAdapter):
    def __init__(self, name: str = "mock"):
        self._name = name
        self.complete_response = Response(
            id="resp_1", model="mock-model", provider=name,
            message=Message.assistant("Mock response"),
            finish_reason=FinishReason.STOP,
            usage=Usage(input_tokens=10, output_tokens=5),
        )

    @property
    def provider_name(self) -> str:
        return self._name

    async def complete(self, request: Request) -> Response:
        return self.complete_response

    async def stream(self, request):
        yield  # Empty stream


def test_client_creation():
    adapter = MockAdapter("test")
    client = Client(providers={"test": adapter}, default_provider="test")
    assert client._default_provider == "test"


@pytest.mark.asyncio
async def test_client_complete():
    adapter = MockAdapter("test")
    client = Client(providers={"test": adapter})
    request = Request(model="test-model", messages=[Message.user("hi")], provider="test")
    response = await client.complete(request)
    assert response.text == "Mock response"
    assert response.provider == "test"


@pytest.mark.asyncio
async def test_client_no_provider_raises():
    client = Client(providers={})
    request = Request(model="test-model", messages=[Message.user("hi")])
    with pytest.raises(ConfigurationError):
        await client.complete(request)


@pytest.mark.asyncio
async def test_client_middleware():
    adapter = MockAdapter("test")

    class CountingMiddleware(Middleware):
        def __init__(self):
            self.count = 0

        async def __call__(self, request, next_fn):
            self.count += 1
            return await next_fn(request)

    mw = CountingMiddleware()
    client = Client(providers={"test": adapter}, middleware=[mw])
    request = Request(model="test", messages=[Message.user("hi")], provider="test")
    await client.complete(request)
    assert mw.count == 1
