"""Handler protocol and outcome model."""

from __future__ import annotations

from abc import ABC, abstractmethod
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field

from ..context import PipelineContext
from ..graph import Graph, Node


class Outcome(BaseModel):
    """Result of executing a node handler."""
    status: str = "success"  # success, retry, fail, partial_success
    preferred_label: str = ""
    suggested_next_ids: list[str] = Field(default_factory=list)
    context_updates: dict[str, Any] = Field(default_factory=dict)
    notes: str = ""


class Handler(ABC):
    """Protocol for node handlers."""

    @abstractmethod
    async def execute(
        self,
        node: Node,
        context: PipelineContext,
        graph: Graph,
        run_dir: Path,
    ) -> Outcome: ...
