"""Core agentic turn loop."""

from __future__ import annotations

import json
import logging
import time
from typing import Any

from pydantic import BaseModel, Field

from ..llm.client import Client
from ..llm.models import (
    ContentKind, ContentPart, Message, Request, Response, Role,
)
from .events import Event, EventBus, EventType
from .history import ConversationHistory, TruncationStrategy
from .loop_detector import LoopDetector
from .steering import SteeringManager
from .tools.base import ToolRegistry

logger = logging.getLogger(__name__)


class SessionConfig(BaseModel):
    max_turns: int = 50
    max_tokens: int | None = None
    model: str = "claude-sonnet-4-5"
    provider: str | None = None
    temperature: float | None = None
    reasoning_effort: str | None = None
    truncation_strategy: TruncationStrategy = TruncationStrategy.SLIDING_WINDOW
    max_context_tokens: int = 100000


class TurnResult(BaseModel):
    messages: list[Message] = Field(default_factory=list)
    final_response: str = ""
    tool_calls_made: int = 0
    turns_used: int = 0


class AgentLoop:
    """Core agentic turn loop: user input → LLM → tools → repeat."""

    def __init__(
        self,
        client: Client,
        tool_registry: ToolRegistry,
        event_bus: EventBus,
        loop_detector: LoopDetector,
        steering: SteeringManager,
    ):
        self._client = client
        self._tools = tool_registry
        self._events = event_bus
        self._loop_detector = loop_detector
        self._steering = steering

    async def run_turn(
        self,
        history: ConversationHistory,
        system_prompt: str,
        config: SessionConfig,
    ) -> TurnResult:
        """Run a complete turn: LLM call → tool execution → repeat until text response."""
        result = TurnResult()
        total_tool_calls = 0

        for turn in range(config.max_turns):
            result.turns_used = turn + 1
            await self._events.emit(Event.turn_start(turn + 1))

            # Inject steering messages
            steering_msgs = self._steering.drain_steering()
            for msg in steering_msgs:
                history.add(msg)
                result.messages.append(msg)
                await self._events.emit(
                    Event(type=EventType.STEERING_INJECTED, data={"text": msg.text})
                )

            # Truncate if needed
            history.truncate(config.truncation_strategy, config.max_context_tokens)

            # Build request
            messages = [Message.system(system_prompt)] + history.messages
            tool_defs = self._tools.list_definitions()

            request = Request(
                model=config.model,
                messages=messages,
                provider=config.provider,
                tools=tool_defs if tool_defs else None,
                temperature=config.temperature,
                max_tokens=config.max_tokens,
                reasoning_effort=config.reasoning_effort,
            )

            await self._events.emit(
                Event.llm_request(config.model, len(messages))
            )

            # Call LLM
            response = await self._client.complete(request)

            await self._events.emit(Event.llm_response(
                response.model,
                response.finish_reason.value,
                response.usage.total_tokens,
            ))

            # Emit thinking events
            for part in response.message.content:
                if part.kind == ContentKind.THINKING and part.thinking:
                    await self._events.emit(Event.thinking(part.thinking.text))

            # Add assistant message to history
            history.add(response.message)
            result.messages.append(response.message)

            # Check for tool calls
            tool_calls = response.tool_calls
            if not tool_calls:
                # Text-only response — turn is complete
                result.final_response = response.text
                await self._events.emit(
                    Event.turn_end(turn + 1, total_tool_calls)
                )
                return result

            # Execute tool calls
            from .environment import ExecutionEnvironment
            for tc in tool_calls:
                args = tc.arguments if isinstance(tc.arguments, dict) else {}

                await self._events.emit(Event.tool_call_start(tc.name, args))
                start_time = time.monotonic()

                tool_result = await self._tools.execute(
                    tc.name, args, self._execution_env,
                )

                elapsed = (time.monotonic() - start_time) * 1000
                total_tool_calls += 1

                await self._events.emit(Event.tool_call_end(
                    tc.name, not tool_result.is_error, elapsed,
                ))

                if tool_result.is_error:
                    await self._events.emit(
                        Event.tool_error(tc.name, tool_result.content)
                    )

                # Add tool result message
                tool_msg = Message.tool_result(
                    tc.id, tool_result.content, tool_result.is_error,
                )
                history.add(tool_msg)
                result.messages.append(tool_msg)

                # Check loop detector
                detection = self._loop_detector.record(tc.name, args)
                if detection.is_looping:
                    await self._events.emit(Event(
                        type=EventType.LOOP_DETECTED,
                        data={"description": detection.description},
                    ))
                    nudge = Message.user(
                        "You seem to be stuck in a loop. "
                        "Please try a different approach or explain what's blocking you."
                    )
                    history.add(nudge)
                    result.messages.append(nudge)

        result.tool_calls_made = total_tool_calls
        result.final_response = (
            f"Reached maximum turns ({config.max_turns}). Last response may be incomplete."
        )
        return result

    def set_execution_env(self, env: Any) -> None:
        self._execution_env = env
