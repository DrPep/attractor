"""5-phase pipeline lifecycle runner."""

from __future__ import annotations

import asyncio
import logging
import uuid
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable

from pydantic import BaseModel, Field

from ..exceptions import GoalGateError, PipelineError
from ..llm.client import Client
from .checkpoint import Checkpoint, load_checkpoint, save_checkpoint
from .context import PipelineContext
from .edge_selector import select_edge
from .goal_gate import check_goal_gates
from .graph import Graph, Node, NodeType
from .handlers import HandlerRegistry, default_registry
from .handlers.base import Outcome
from .interviewer import AutoApproveInterviewer, Interviewer
from .parser import parse_dot
from .retry import RetryPolicy, compute_delay, is_retryable, RETRY_STANDARD
from .status import NodeStatus, ensure_run_dir, read_status, write_status
from .stylesheet import apply_stylesheet, parse_stylesheet
from .validation import validate_or_raise

logger = logging.getLogger(__name__)


class RunResult(BaseModel):
    """Result of a pipeline run."""
    success: bool = False
    run_id: str = ""
    nodes_executed: list[str] = Field(default_factory=list)
    node_outcomes: dict[str, str] = Field(default_factory=dict)
    final_context: dict[str, Any] = Field(default_factory=dict)
    errors: list[str] = Field(default_factory=list)
    run_dir: str = ""


class PipelineRunner:
    """Main orchestrator: parses DOT, validates, executes graph, manages state."""

    def __init__(
        self,
        client: Client | None = None,
        handler_registry: HandlerRegistry | None = None,
        interviewer: Interviewer | None = None,
        on_node_start: Callable[[str], Any] | None = None,
        on_node_end: Callable[[str, str], Any] | None = None,
    ):
        self._client = client
        self._interviewer = interviewer or AutoApproveInterviewer()
        self._registry = handler_registry or default_registry(
            client=client, interviewer=self._interviewer,
        )
        self._on_node_start = on_node_start
        self._on_node_end = on_node_end

    async def run(
        self,
        dot_source: str | Path,
        run_dir: Path | None = None,
        resume: bool = False,
    ) -> RunResult:
        """Run a pipeline through all 5 phases."""
        run_id = uuid.uuid4().hex[:12]
        if run_dir is None:
            run_dir = Path(f"runs/{run_id}")

        result = RunResult(run_id=run_id, run_dir=str(run_dir))

        try:
            # Phase 1: PARSE
            logger.info("Phase 1: PARSE")
            graph = parse_dot(dot_source)

            # Phase 2: VALIDATE
            logger.info("Phase 2: VALIDATE")
            warnings = validate_or_raise(graph)
            for w in warnings:
                logger.warning("Validation: %s", w.message)

            # Phase 3: INITIALIZE
            logger.info("Phase 3: INITIALIZE")
            ensure_run_dir(run_dir)
            context = PipelineContext()
            context.set("graph.goal", graph.goal)
            context.set("run_id", run_id)

            # Apply stylesheet
            if graph.model_stylesheet:
                rules = parse_stylesheet(graph.model_stylesheet)
                apply_stylesheet(rules, graph)

            # Check for checkpoint resume
            completed_nodes: list[str] = []
            node_outcomes: dict[str, str] = {}
            node_retries: dict[str, int] = {}
            start_node_id = ""

            if resume:
                checkpoint = load_checkpoint(run_dir)
                if checkpoint:
                    logger.info("Resuming from checkpoint at node %s", checkpoint.current_node)
                    context.restore(checkpoint.context_snapshot)
                    completed_nodes = list(checkpoint.completed_nodes)
                    node_retries = dict(checkpoint.node_retries)
                    start_node_id = checkpoint.current_node
                    # Rebuild outcomes from status files
                    for nid in completed_nodes:
                        status = read_status(nid, run_dir)
                        if status:
                            node_outcomes[nid] = status.outcome

            if not start_node_id:
                start = graph.start_node
                if not start:
                    raise PipelineError("No start node found")
                start_node_id = start.id

            # Phase 4: EXECUTE
            logger.info("Phase 4: EXECUTE")
            current_node_id = start_node_id

            max_iterations = 1000  # Safety limit
            iteration = 0

            while iteration < max_iterations:
                iteration += 1
                node = graph.get_node(current_node_id)
                if not node:
                    result.errors.append(f"Node not found: {current_node_id}")
                    break

                context.set("current_node", current_node_id)

                if self._on_node_start:
                    self._on_node_start(current_node_id)

                logger.info("Executing node: %s (%s)", node.id, node.type.value)

                # Execute handler with retry support
                outcome = await self._execute_with_retry(
                    node, context, graph, run_dir, node_retries,
                )

                # Record completion
                completed_nodes.append(node.id)
                node_outcomes[node.id] = outcome.status
                result.nodes_executed.append(node.id)
                result.node_outcomes[node.id] = outcome.status

                # Apply context updates
                if outcome.context_updates:
                    context.update(outcome.context_updates)

                # Write status
                write_status(node.id, NodeStatus(
                    outcome=outcome.status,
                    preferred_next_label=outcome.preferred_label,
                    suggested_next_ids=outcome.suggested_next_ids,
                    context_updates=outcome.context_updates,
                    notes=outcome.notes,
                ), run_dir)

                # Save checkpoint
                save_checkpoint(Checkpoint(
                    run_id=run_id,
                    current_node=current_node_id,
                    completed_nodes=completed_nodes,
                    node_retries=node_retries,
                    context_snapshot=context.snapshot(),
                ), run_dir)

                if self._on_node_end:
                    self._on_node_end(current_node_id, outcome.status)

                # Check if we've reached an exit node
                if node.type == NodeType.EXIT:
                    # Check goal gates before accepting exit
                    gate_result = check_goal_gates(graph, node_outcomes)
                    if not gate_result.satisfied:
                        if gate_result.retry_target:
                            logger.warning(
                                "Goal gates unsatisfied: %s. Routing to %s",
                                gate_result.unsatisfied_gates,
                                gate_result.retry_target,
                            )
                            current_node_id = gate_result.retry_target
                            continue
                        else:
                            result.errors.append(
                                f"Goal gates unsatisfied: {gate_result.unsatisfied_gates}"
                            )
                            break
                    # All gates satisfied
                    result.success = True
                    break

                # Select next edge
                outgoing = graph.get_outgoing_edges(node.id)
                if not outgoing:
                    logger.info("No outgoing edges from %s, pipeline ends", node.id)
                    result.success = outcome.status == "success"
                    break

                next_edge = select_edge(
                    outgoing,
                    outcome=outcome.status,
                    preferred_label=outcome.preferred_label,
                    suggested_next_ids=outcome.suggested_next_ids,
                    context=context,
                )

                if not next_edge:
                    result.errors.append(
                        f"No eligible edge from node '{node.id}'"
                    )
                    break

                current_node_id = next_edge.target

            if iteration >= max_iterations:
                result.errors.append("Maximum iteration limit reached")

            # Phase 5: FINALIZE
            logger.info("Phase 5: FINALIZE")
            result.final_context = context.snapshot()

        except Exception as e:
            logger.exception("Pipeline error")
            result.errors.append(str(e))

        return result

    async def _execute_with_retry(
        self,
        node: Node,
        context: PipelineContext,
        graph: Graph,
        run_dir: Path,
        node_retries: dict[str, int],
    ) -> Outcome:
        """Execute a node handler with retry support."""
        handler = self._registry.get(node.type)
        if not handler:
            return Outcome(
                status="fail",
                notes=f"No handler for node type: {node.type.value}",
            )

        max_retries = node.max_retries or 0
        retries = node_retries.get(node.id, 0)

        for attempt in range(max_retries + 1):
            try:
                outcome = await handler.execute(node, context, graph, run_dir)

                if outcome.status == "retry" and attempt < max_retries:
                    retries += 1
                    node_retries[node.id] = retries
                    context.set(f"internal.retry_count.{node.id}", retries)
                    delay = compute_delay(attempt, RETRY_STANDARD)
                    logger.info(
                        "Node %s returned retry, attempt %d/%d (delay %.1fs)",
                        node.id, attempt + 1, max_retries, delay,
                    )
                    await asyncio.sleep(delay)
                    continue

                if outcome.status == "fail" and attempt < max_retries:
                    retries += 1
                    node_retries[node.id] = retries
                    delay = compute_delay(attempt, RETRY_STANDARD)
                    logger.info(
                        "Node %s failed, retrying %d/%d (delay %.1fs)",
                        node.id, attempt + 1, max_retries, delay,
                    )
                    await asyncio.sleep(delay)
                    continue

                # Accept the outcome
                if outcome.status == "fail" and node.allow_partial:
                    outcome.status = "partial_success"

                return outcome

            except Exception as e:
                if attempt < max_retries and is_retryable(e):
                    retries += 1
                    node_retries[node.id] = retries
                    delay = compute_delay(attempt, RETRY_STANDARD)
                    logger.warning(
                        "Node %s error (attempt %d/%d): %s",
                        node.id, attempt + 1, max_retries, e,
                    )
                    await asyncio.sleep(delay)
                    continue
                return Outcome(status="fail", notes=str(e))

        return Outcome(status="fail", notes="Exhausted all retries")
