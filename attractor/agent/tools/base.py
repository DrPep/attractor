"""Tool protocol, registry, and result types."""

from __future__ import annotations

from abc import ABC, abstractmethod
from typing import Any, TYPE_CHECKING

from pydantic import BaseModel, Field

if TYPE_CHECKING:
    from ..environment import ExecutionEnvironment

from ...llm.models import ToolDefinition


class ToolResult(BaseModel):
    content: str = ""
    is_error: bool = False
    metadata: dict[str, Any] = Field(default_factory=dict)


class Tool(ABC):
    """Base class for agent tools."""

    @property
    @abstractmethod
    def name(self) -> str: ...

    @property
    @abstractmethod
    def description(self) -> str: ...

    @property
    @abstractmethod
    def parameters_schema(self) -> dict[str, Any]: ...

    @abstractmethod
    async def execute(
        self, args: dict[str, Any], env: ExecutionEnvironment,
    ) -> ToolResult: ...

    def to_definition(self) -> ToolDefinition:
        return ToolDefinition(
            name=self.name,
            description=self.description,
            parameters=self.parameters_schema,
        )


class ToolRegistry:
    """Registry mapping tool names to tool instances."""

    def __init__(self) -> None:
        self._tools: dict[str, Tool] = {}

    def register(self, tool: Tool) -> None:
        self._tools[tool.name] = tool

    def remove(self, name: str) -> None:
        """Remove a tool by name. No-op if the tool is not registered."""
        self._tools.pop(name, None)

    def get(self, name: str) -> Tool | None:
        return self._tools.get(name)

    def list_tools(self) -> list[Tool]:
        return list(self._tools.values())

    def list_definitions(self) -> list[ToolDefinition]:
        return [t.to_definition() for t in self._tools.values()]

    async def execute(
        self, name: str, args: dict[str, Any], env: ExecutionEnvironment,
    ) -> ToolResult:
        tool = self._tools.get(name)
        if not tool:
            return ToolResult(
                content=f"Unknown tool: {name}", is_error=True,
            )
        try:
            return await tool.execute(args, env)
        except Exception as e:
            return ToolResult(content=f"Tool error: {e}", is_error=True)
