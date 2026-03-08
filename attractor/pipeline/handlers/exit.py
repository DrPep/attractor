"""Exit node handler - no-op (goal gate check done by engine)."""

from __future__ import annotations

from pathlib import Path

from ..context import PipelineContext
from ..graph import Graph, Node
from .base import Handler, Outcome


class ExitHandler(Handler):
    async def execute(
        self, node: Node, context: PipelineContext,
        graph: Graph, run_dir: Path,
    ) -> Outcome:
        return Outcome(status="success", notes="Pipeline exit reached")
