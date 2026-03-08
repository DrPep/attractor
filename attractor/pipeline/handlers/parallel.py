"""Parallel execution handler."""

from __future__ import annotations

import asyncio
import logging
from pathlib import Path
from typing import Any

from ..context import PipelineContext
from ..graph import Graph, Node
from .base import Handler, Outcome

logger = logging.getLogger(__name__)


class ParallelHandler(Handler):
    """Runs target node handlers concurrently."""

    def __init__(self, handler_registry: Any = None):
        self._registry = handler_registry

    async def execute(
        self, node: Node, context: PipelineContext,
        graph: Graph, run_dir: Path,
    ) -> Outcome:
        outgoing = graph.get_outgoing_edges(node.id)
        if not outgoing:
            return Outcome(status="success", notes="No branches to execute")

        join_policy = str(node.attrs.get("join_policy", "wait_all"))

        # Execute branches concurrently
        async def run_branch(edge_target: str) -> tuple[str, Outcome]:
            target_node = graph.get_node(edge_target)
            if not target_node or not self._registry:
                return edge_target, Outcome(status="fail", notes="No handler")

            handler = self._registry.get(target_node.type)
            if not handler:
                return edge_target, Outcome(status="fail", notes=f"No handler for {target_node.type}")

            # Create isolated context for branch
            branch_ctx = PipelineContext(context.snapshot())
            outcome = await handler.execute(target_node, branch_ctx, graph, run_dir)
            return edge_target, outcome

        tasks = [run_branch(e.target) for e in outgoing]

        if join_policy == "first_success":
            # Return on first success
            done, pending = await asyncio.wait(
                [asyncio.create_task(t) for t in tasks],
                return_when=asyncio.FIRST_COMPLETED,
            )
            results = {}
            for task in done:
                target, outcome = await task
                results[target] = outcome
                if outcome.status == "success":
                    for p in pending:
                        p.cancel()
                    return Outcome(
                        status="success",
                        notes=f"First success from {target}",
                        context_updates={"parallel.results": {target: outcome.status}},
                    )
            # None succeeded from completed, wait for rest
            for p in pending:
                try:
                    target, outcome = await p
                    results[target] = outcome
                    if outcome.status == "success":
                        return Outcome(
                            status="success",
                            notes=f"First success from {target}",
                        )
                except asyncio.CancelledError:
                    pass
            return Outcome(status="fail", notes="No branch succeeded")
        else:
            # wait_all (default)
            results: dict[str, str] = {}
            all_outcomes = await asyncio.gather(*tasks, return_exceptions=True)
            for item in all_outcomes:
                if isinstance(item, Exception):
                    logger.error("Branch error: %s", item)
                    continue
                target, outcome = item
                results[target] = outcome.status

            all_success = all(s == "success" for s in results.values())
            return Outcome(
                status="success" if all_success else "partial_success",
                notes=f"Parallel: {len(results)} branches completed",
                context_updates={"parallel.results": results},
            )
