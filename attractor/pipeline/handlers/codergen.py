"""Codergen handler - invokes LLM via the agent loop."""

from __future__ import annotations

import logging
from pathlib import Path
from typing import Any, Callable

from ...llm.client import Client
from ...agent.session import Session
from ...agent.skill import SkillRegistry
from ...agent.provider_profile import ProviderProfile, CODING_AGENT_SYSTEM_PROMPT
from ...agent.loop import SessionConfig
from ..context import PipelineContext
from ..graph import Graph, Node
from ..status import write_prompt, write_response
from .base import Handler, Outcome

logger = logging.getLogger(__name__)


class CodergenHandler(Handler):
    """Expands $goal in prompt, calls LLM backend, writes artifacts."""

    def __init__(
        self,
        client: Client | None = None,
        skill_registry: SkillRegistry | None = None,
        model_override: str | None = None,
        provider_override: str | None = None,
        on_agent_event: Callable[[Any], Any] | None = None,
    ):
        self._client = client
        self._skill_registry = skill_registry or SkillRegistry()
        self._model_override = model_override
        self._provider_override = provider_override
        self._on_agent_event = on_agent_event

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

        # If this node has run before, inject prior feedback from context.
        # Downstream nodes store their output as "{node_id}.response" in context;
        # a feedback edge routes us back here with that data available.
        iteration = int(context.get(f"internal.node_iteration.{node.id}", 0))
        if iteration > 0:
            feedback_parts: list[str] = []
            # Gather responses from all downstream nodes that have run
            for key in context.keys():
                if key.endswith(".response") and key != f"{node.id}.response":
                    feedback_parts.append(f"[{key}]\n{context.get(key)}")
            if feedback_parts:
                prompt = (
                    prompt
                    + "\n\n--- Feedback from previous iteration ---\n"
                    + "\n\n".join(feedback_parts)
                    + "\n\nPlease address the issues identified above."
                )
        context.set(f"internal.node_iteration.{node.id}", iteration + 1)

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
            # Build provider profile: CLI override > node attribute > default
            model = self._model_override or node.llm_model or "claude-sonnet-4-6"
            provider = self._provider_override or node.llm_provider or None

            # Resolve skills and compose system prompt / tool set
            system_prompt = CODING_AGENT_SYSTEM_PROMPT
            tool_registry = None

            if node.skills:
                composed = self._skill_registry.compose(node.skills)
                if composed.system_prompt:
                    system_prompt = system_prompt + "\n\n" + composed.system_prompt
                tool_registry = self._skill_registry.build_tool_registry(composed)

            profile = ProviderProfile(
                provider_name=provider or "anthropic",
                model=model,
                system_prompt=system_prompt,
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
                tool_registry=tool_registry,
            )

            if self._on_agent_event:
                session.on_all_events(self._on_agent_event)

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
