"""Conditional handler - pass-through for edge condition evaluation."""

from __future__ import annotations

from pathlib import Path

from ..context import PipelineContext
from ..graph import Graph, Node
from .base import Handler, Outcome


class ConditionalHandler(Handler):
    """Pass-through handler. Edge conditions are evaluated by the edge selector."""

    async def execute(
        self, node: Node, context: PipelineContext,
        graph: Graph, run_dir: Path,
    ) -> Outcome:
        return Outcome(
            status="success",
            notes="Conditional node - routing determined by edge conditions",
        )
