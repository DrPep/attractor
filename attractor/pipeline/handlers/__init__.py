"""Handler registry with built-in handlers."""

from __future__ import annotations

from typing import Any

from ..graph import NodeType
from .base import Handler, Outcome
from .start import StartHandler
from .exit import ExitHandler
from .codergen import CodergenHandler
from .wait_human import WaitHumanHandler
from .conditional import ConditionalHandler
from .parallel import ParallelHandler
from .tool import ToolHandler


class HandlerRegistry:
    """Registry mapping node types to handler instances."""

    def __init__(self) -> None:
        self._handlers: dict[NodeType, Handler] = {}

    def register(self, node_type: NodeType, handler: Handler) -> None:
        self._handlers[node_type] = handler

    def get(self, node_type: NodeType) -> Handler | None:
        return self._handlers.get(node_type)


def default_registry(
    client: Any = None,
    interviewer: Any = None,
    skill_registry: Any = None,
    model_override: str | None = None,
    provider_override: str | None = None,
    on_agent_event: Any = None,
) -> HandlerRegistry:
    """Create a HandlerRegistry with all built-in handlers."""
    from ..interviewer import AutoApproveInterviewer

    registry = HandlerRegistry()
    registry.register(NodeType.START, StartHandler())
    registry.register(NodeType.EXIT, ExitHandler())
    registry.register(NodeType.CODERGEN, CodergenHandler(
        client=client, skill_registry=skill_registry,
        model_override=model_override, provider_override=provider_override,
        on_agent_event=on_agent_event,
    ))
    registry.register(
        NodeType.WAIT_HUMAN,
        WaitHumanHandler(interviewer or AutoApproveInterviewer()),
    )
    registry.register(NodeType.CONDITIONAL, ConditionalHandler())

    parallel = ParallelHandler(handler_registry=registry)
    registry.register(NodeType.PARALLEL, parallel)
    registry.register(NodeType.TOOL, ToolHandler())

    return registry


__all__ = [
    "Handler",
    "HandlerRegistry",
    "Outcome",
    "default_registry",
    "StartHandler",
    "ExitHandler",
    "CodergenHandler",
    "WaitHumanHandler",
    "ConditionalHandler",
    "ParallelHandler",
    "ToolHandler",
]
