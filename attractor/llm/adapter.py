"""Provider adapter protocol."""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import AsyncIterator

from .models import Request, Response
from .streaming import StreamEvent


class ProviderAdapter(ABC):
    """Interface every LLM provider adapter must implement."""

    @property
    @abstractmethod
    def provider_name(self) -> str: ...

    @abstractmethod
    async def complete(self, request: Request) -> Response: ...

    @abstractmethod
    async def stream(self, request: Request) -> AsyncIterator[StreamEvent]: ...

    async def close(self) -> None:
        """Release resources."""

    async def initialize(self) -> None:
        """Validate configuration on startup."""

    def supports_tool_choice(self, mode: str) -> bool:
        return True
