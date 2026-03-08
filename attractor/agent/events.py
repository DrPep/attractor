"""Event system for the coding agent loop."""

from __future__ import annotations

import asyncio
import logging
from enum import Enum
from typing import Any, Callable, Awaitable

from pydantic import BaseModel, Field

logger = logging.getLogger(__name__)


class EventType(str, Enum):
    TURN_START = "turn_start"
    TURN_END = "turn_end"
    LLM_REQUEST = "llm_request"
    LLM_RESPONSE = "llm_response"
    LLM_STREAM_DELTA = "llm_stream_delta"
    TOOL_CALL_START = "tool_call_start"
    TOOL_CALL_END = "tool_call_end"
    TOOL_ERROR = "tool_error"
    THINKING = "thinking"
    TEXT_DELTA = "text_delta"
    SESSION_START = "session_start"
    SESSION_END = "session_end"
    STEERING_INJECTED = "steering_injected"
    LOOP_DETECTED = "loop_detected"
    HISTORY_TRUNCATED = "history_truncated"
    ERROR = "error"


class Event(BaseModel):
    type: EventType
    data: dict[str, Any] = Field(default_factory=dict)

    @classmethod
    def turn_start(cls, turn_number: int) -> Event:
        return cls(type=EventType.TURN_START, data={"turn_number": turn_number})

    @classmethod
    def turn_end(cls, turn_number: int, tool_calls: int) -> Event:
        return cls(type=EventType.TURN_END, data={
            "turn_number": turn_number, "tool_calls": tool_calls,
        })

    @classmethod
    def llm_request(cls, model: str, message_count: int) -> Event:
        return cls(type=EventType.LLM_REQUEST, data={
            "model": model, "message_count": message_count,
        })

    @classmethod
    def llm_response(cls, model: str, finish_reason: str, tokens: int) -> Event:
        return cls(type=EventType.LLM_RESPONSE, data={
            "model": model, "finish_reason": finish_reason, "tokens": tokens,
        })

    @classmethod
    def tool_call_start(cls, tool_name: str, args: dict[str, Any]) -> Event:
        return cls(type=EventType.TOOL_CALL_START, data={
            "tool_name": tool_name, "args": args,
        })

    @classmethod
    def tool_call_end(
        cls, tool_name: str, success: bool, duration_ms: float,
    ) -> Event:
        return cls(type=EventType.TOOL_CALL_END, data={
            "tool_name": tool_name, "success": success,
            "duration_ms": duration_ms,
        })

    @classmethod
    def tool_error(cls, tool_name: str, error: str) -> Event:
        return cls(type=EventType.TOOL_ERROR, data={
            "tool_name": tool_name, "error": error,
        })

    @classmethod
    def thinking(cls, text: str) -> Event:
        return cls(type=EventType.THINKING, data={"text": text})

    @classmethod
    def text_delta(cls, text: str) -> Event:
        return cls(type=EventType.TEXT_DELTA, data={"text": text})

    @classmethod
    def error(cls, error: str, error_type: str = "unknown") -> Event:
        return cls(type=EventType.ERROR, data={
            "error": error, "error_type": error_type,
        })


EventHandler = Callable[[Event], Any]


class EventBus:
    """Event bus supporting both sync and async handlers."""

    def __init__(self) -> None:
        self._handlers: dict[EventType, list[EventHandler]] = {}
        self._global_handlers: list[EventHandler] = []

    def on(self, event_type: EventType, handler: EventHandler) -> None:
        self._handlers.setdefault(event_type, []).append(handler)

    def on_all(self, handler: EventHandler) -> None:
        self._global_handlers.append(handler)

    async def emit(self, event: Event) -> None:
        handlers = self._global_handlers + self._handlers.get(event.type, [])
        for handler in handlers:
            try:
                result = handler(event)
                if asyncio.iscoroutine(result):
                    await result
            except Exception:
                logger.exception("Error in event handler for %s", event.type)
