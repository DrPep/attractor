"""Streaming event types and accumulator."""

from __future__ import annotations

from enum import Enum
from typing import Any

from pydantic import BaseModel, Field

from .models import (
    ContentKind,
    ContentPart,
    FinishReason,
    Message,
    Response,
    Role,
    ToolCallData,
    Usage,
)


class StreamEventType(str, Enum):
    MESSAGE_START = "message_start"
    CONTENT_DELTA = "content_delta"
    TOOL_CALL_START = "tool_call_start"
    TOOL_CALL_DELTA = "tool_call_delta"
    THINKING_DELTA = "thinking_delta"
    MESSAGE_END = "message_end"
    ERROR = "error"


class StreamEvent(BaseModel):
    type: StreamEventType
    # MessageStart fields
    id: str | None = None
    model: str | None = None
    # ContentDelta fields
    text: str | None = None
    index: int = 0
    # ToolCallStart fields
    tool_call_id: str | None = None
    tool_name: str | None = None
    # ToolCallDelta fields
    arguments_chunk: str | None = None
    # MessageEnd fields
    finish_reason: FinishReason | None = None
    usage: Usage | None = None
    # Error fields
    error_type: str | None = None
    error_message: str | None = None

    @classmethod
    def message_start(cls, id: str, model: str) -> StreamEvent:
        return cls(type=StreamEventType.MESSAGE_START, id=id, model=model)

    @classmethod
    def content_delta(cls, text: str, index: int = 0) -> StreamEvent:
        return cls(type=StreamEventType.CONTENT_DELTA, text=text, index=index)

    @classmethod
    def tool_call_start(cls, id: str, name: str, index: int = 0) -> StreamEvent:
        return cls(
            type=StreamEventType.TOOL_CALL_START,
            tool_call_id=id, tool_name=name, index=index,
        )

    @classmethod
    def tool_call_delta(cls, chunk: str, index: int = 0) -> StreamEvent:
        return cls(
            type=StreamEventType.TOOL_CALL_DELTA,
            arguments_chunk=chunk, index=index,
        )

    @classmethod
    def thinking_delta(cls, text: str, index: int = 0) -> StreamEvent:
        return cls(type=StreamEventType.THINKING_DELTA, text=text, index=index)

    @classmethod
    def message_end(
        cls, finish_reason: FinishReason, usage: Usage | None = None,
    ) -> StreamEvent:
        return cls(
            type=StreamEventType.MESSAGE_END,
            finish_reason=finish_reason, usage=usage,
        )

    @classmethod
    def error(cls, error_type: str, message: str) -> StreamEvent:
        return cls(
            type=StreamEventType.ERROR,
            error_type=error_type, error_message=message,
        )


class StreamAccumulator:
    """Collects stream deltas into a full Response."""

    def __init__(self) -> None:
        self.id: str = ""
        self.model: str = ""
        self.provider: str = ""
        self.text_parts: dict[int, list[str]] = {}
        self.tool_calls: dict[int, dict[str, Any]] = {}
        self.thinking_parts: dict[int, list[str]] = {}
        self.finish_reason: FinishReason = FinishReason.STOP
        self.usage: Usage = Usage()

    def process(self, event: StreamEvent) -> None:
        t = event.type
        if t == StreamEventType.MESSAGE_START:
            self.id = event.id or ""
            self.model = event.model or ""
        elif t == StreamEventType.CONTENT_DELTA:
            self.text_parts.setdefault(event.index, []).append(event.text or "")
        elif t == StreamEventType.TOOL_CALL_START:
            self.tool_calls[event.index] = {
                "id": event.tool_call_id or "",
                "name": event.tool_name or "",
                "arguments_chunks": [],
            }
        elif t == StreamEventType.TOOL_CALL_DELTA:
            if event.index in self.tool_calls:
                self.tool_calls[event.index]["arguments_chunks"].append(
                    event.arguments_chunk or ""
                )
        elif t == StreamEventType.THINKING_DELTA:
            self.thinking_parts.setdefault(event.index, []).append(event.text or "")
        elif t == StreamEventType.MESSAGE_END:
            self.finish_reason = event.finish_reason or FinishReason.STOP
            if event.usage:
                self.usage = event.usage

    def to_response(self) -> Response:
        parts: list[ContentPart] = []

        for idx in sorted(self.thinking_parts):
            text = "".join(self.thinking_parts[idx])
            parts.append(ContentPart.thinking_part(text))

        for idx in sorted(self.text_parts):
            text = "".join(self.text_parts[idx])
            parts.append(ContentPart.text_part(text))

        for idx in sorted(self.tool_calls):
            tc = self.tool_calls[idx]
            import json
            args_str = "".join(tc["arguments_chunks"])
            try:
                arguments = json.loads(args_str) if args_str else {}
            except json.JSONDecodeError:
                arguments = args_str
            parts.append(ContentPart.tool_call_part(tc["id"], tc["name"], arguments))

        if self.tool_calls:
            finish_reason = FinishReason.TOOL_USE
        else:
            finish_reason = self.finish_reason

        return Response(
            id=self.id,
            model=self.model,
            provider=self.provider,
            message=Message(role=Role.ASSISTANT, content=parts),
            finish_reason=finish_reason,
            usage=self.usage,
        )
