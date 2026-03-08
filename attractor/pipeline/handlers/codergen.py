"""Codergen handler - invokes LLM via the agent loop."""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any

from ...llm.client import Client
from ...agent.session import Session
from ...agent.provider_profile import ProviderProfile
from ...agent.loop import SessionConfig
from ..context import PipelineContext
from ..graph import Graph, Node
from ..status import write_prompt, write_response
from .base import Handler, Outcome

logger = logging.getLogger(__name__)


class CodergenHandler(Handler):
    """Expands $goal in prompt, calls LLM backend, writes artifacts."""

    def __init__(self, client: Client | None = None):
        self._client = client

    async def execute(
        self, node: Node, context: PipelineContext,
        graph: Graph, run_dir: Path,
    ) -> Outcome:
        # Expand $goal in prompt
        prompt = node.prompt
        goal = context.get("graph.goal", graph.goal) or graph.goal
        if "$goal" in prompt:
            prompt = prompt.replace("$goal", goal)

        if not prompt:
            prompt = goal or f"Execute task: {node.label}"

        # Write prompt artifact
        write_prompt(node.id, prompt, run_dir)

        if not self._client:
            # No LLM client - write a placeholder
            write_response(node.id, "(no LLM client configured)", run_dir)
            return Outcome(
                status="success" if node.auto_status else "fail",
                notes="No LLM client configured",
            )

        try:
            # Build provider profile from node attributes
            model = node.llm_model or "claude-sonnet-4-5"
            provider = node.llm_provider or None

            profile = ProviderProfile(
                provider_name=provider or "anthropic",
                model=model,
                reasoning_effort=node.reasoning_effort or None,
            )

            config = SessionConfig(
                model=model,
                provider=provider,
                reasoning_effort=node.reasoning_effort or None,
            )

            session = Session(
                client=self._client,
                profile=profile,
                config=config,
            )

            result = await session.submit(prompt)
            response_text = result.final_response

            # Write response artifact
            write_response(node.id, response_text, run_dir)

            return Outcome(
                status="success",
                notes=f"Completed with {result.tool_calls_made} tool calls in {result.turns_used} turns",
                context_updates={f"{node.id}.response": response_text},
            )
        except Exception as e:
            logger.exception("Codergen handler failed for node %s", node.id)
            write_response(node.id, f"Error: {e}", run_dir)

            if node.auto_status:
                return Outcome(status="success", notes=f"Auto-status: {e}")
            return Outcome(status="fail", notes=str(e))
