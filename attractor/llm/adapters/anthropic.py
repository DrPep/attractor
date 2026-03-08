"""Anthropic adapter using the Messages API."""

from __future__ import annotations

import json
from typing import Any, AsyncIterator

from ..adapter import ProviderAdapter
from ..models import (
    ContentKind, ContentPart, FinishReason, Message, Request, Response,
    Role, ToolCallData, ToolDefinition, Usage,
)
from ..streaming import StreamEvent


class AnthropicAdapter(ProviderAdapter):
    """Adapter for Anthropic using the Messages API."""

    def __init__(
        self,
        api_key: str,
        base_url: str | None = None,
    ):
        self._api_key = api_key
        self._base_url = base_url
        self._client: Any = None

    @property
    def provider_name(self) -> str:
        return "anthropic"

    def _get_client(self) -> Any:
        if self._client is None:
            from anthropic import AsyncAnthropic
            kwargs: dict[str, Any] = {"api_key": self._api_key}
            if self._base_url:
                kwargs["base_url"] = self._base_url
            self._client = AsyncAnthropic(**kwargs)
        return self._client

    def _extract_system(
        self, messages: list[Message],
    ) -> tuple[str | list[dict[str, Any]], list[Message]]:
        """Extract system messages and return (system_content, remaining_messages)."""
        system_parts: list[str] = []
        remaining: list[Message] = []
        for msg in messages:
            if msg.role in (Role.SYSTEM, Role.DEVELOPER):
                system_parts.append(msg.text)
            else:
                remaining.append(msg)
        system = "\n\n".join(system_parts) if system_parts else ""
        return system, remaining

    def _translate_messages(self, messages: list[Message]) -> list[dict[str, Any]]:
        result: list[dict[str, Any]] = []
        for msg in messages:
            if msg.role == Role.USER:
                content = self._translate_content_parts(msg.content, is_user=True)
                result.append({"role": "user", "content": content})
            elif msg.role == Role.ASSISTANT:
                content = self._translate_content_parts(msg.content, is_user=False)
                result.append({"role": "assistant", "content": content})
            elif msg.role == Role.TOOL:
                # Tool results go as user messages with tool_result blocks
                blocks: list[dict[str, Any]] = []
                for p in msg.content:
                    if p.kind == ContentKind.TOOL_RESULT and p.tool_result:
                        content_val = p.tool_result.content
                        if isinstance(content_val, dict):
                            content_val = json.dumps(content_val)
                        blocks.append({
                            "type": "tool_result",
                            "tool_use_id": p.tool_result.tool_call_id,
                            "content": str(content_val),
                            "is_error": p.tool_result.is_error,
                        })
                result.append({"role": "user", "content": blocks})
        return result

    def _translate_content_parts(
        self, parts: list[ContentPart], is_user: bool,
    ) -> list[dict[str, Any]] | str:
        blocks: list[dict[str, Any]] = []
        for p in parts:
            if p.kind == ContentKind.TEXT:
                blocks.append({"type": "text", "text": p.text or ""})
            elif p.kind == ContentKind.IMAGE and p.image and is_user:
                if p.image.url:
                    blocks.append({
                        "type": "image",
                        "source": {"type": "url", "url": p.image.url},
                    })
                elif p.image.data:
                    import base64
                    blocks.append({
                        "type": "image",
                        "source": {
                            "type": "base64",
                            "media_type": p.image.media_type or "image/png",
                            "data": base64.b64encode(p.image.data).decode(),
                        },
                    })
            elif p.kind == ContentKind.TOOL_CALL and p.tool_call and not is_user:
                args = p.tool_call.arguments
                if isinstance(args, str):
                    try:
                        args = json.loads(args)
                    except json.JSONDecodeError:
                        args = {"raw": args}
                blocks.append({
                    "type": "tool_use",
                    "id": p.tool_call.id,
                    "name": p.tool_call.name,
                    "input": args,
                })
            elif p.kind == ContentKind.THINKING and p.thinking and not is_user:
                blocks.append({
                    "type": "thinking",
                    "thinking": p.thinking.text,
                    **({"signature": p.thinking.signature} if p.thinking.signature else {}),
                })
            elif p.kind == ContentKind.REDACTED_THINKING and p.thinking and not is_user:
                blocks.append({
                    "type": "redacted_thinking",
                    "data": p.thinking.text,
                })

        if len(blocks) == 1 and blocks[0].get("type") == "text":
            return blocks[0]["text"]
        return blocks

    def _translate_tools(
        self, tools: list[ToolDefinition] | None,
    ) -> list[dict[str, Any]] | None:
        if not tools:
            return None
        return [
            {
                "name": t.name,
                "description": t.description,
                "input_schema": t.parameters,
            }
            for t in tools
        ]

    def _inject_cache_control(
        self, system: str | list[dict[str, Any]],
        messages: list[dict[str, Any]],
    ) -> tuple[Any, list[dict[str, Any]]]:
        """Inject cache_control breakpoints for prompt caching."""
        # Cache the system prompt
        if isinstance(system, str) and system:
            system = [
                {"type": "text", "text": system, "cache_control": {"type": "ephemeral"}}
            ]

        # Cache the last user message (most common pattern for agentic workloads)
        for i in range(len(messages) - 1, -1, -1):
            msg = messages[i]
            if msg.get("role") == "user":
                content = msg.get("content")
                if isinstance(content, str):
                    msg["content"] = [
                        {"type": "text", "text": content,
                         "cache_control": {"type": "ephemeral"}}
                    ]
                elif isinstance(content, list) and content:
                    content[-1]["cache_control"] = {"type": "ephemeral"}
                break

        return system, messages

    def _parse_response(self, raw: Any) -> Response:
        content_parts: list[ContentPart] = []

        for block in raw.content:
            if block.type == "text":
                content_parts.append(ContentPart.text_part(block.text))
            elif block.type == "tool_use":
                content_parts.append(ContentPart.tool_call_part(
                    id=block.id,
                    name=block.name,
                    arguments=block.input if isinstance(block.input, dict) else {},
                ))
            elif block.type == "thinking":
                content_parts.append(ContentPart.thinking_part(
                    text=getattr(block, "thinking", ""),
                    signature=getattr(block, "signature", None),
                ))
            elif block.type == "redacted_thinking":
                content_parts.append(ContentPart.thinking_part(
                    text=getattr(block, "data", ""),
                    redacted=True,
                ))

        stop_reason_map = {
            "end_turn": FinishReason.STOP,
            "tool_use": FinishReason.TOOL_USE,
            "max_tokens": FinishReason.MAX_TOKENS,
            "stop_sequence": FinishReason.STOP,
        }
        finish = stop_reason_map.get(raw.stop_reason, FinishReason.STOP)

        usage = Usage(
            input_tokens=raw.usage.input_tokens,
            output_tokens=raw.usage.output_tokens,
            cache_read_tokens=getattr(raw.usage, "cache_read_input_tokens", 0) or 0,
            cache_write_tokens=getattr(raw.usage, "cache_creation_input_tokens", 0) or 0,
        )

        return Response(
            id=raw.id,
            model=raw.model,
            provider="anthropic",
            message=Message(role=Role.ASSISTANT, content=content_parts),
            finish_reason=finish,
            usage=usage,
        )

    async def complete(self, request: Request) -> Response:
        client = self._get_client()
        system, remaining = self._extract_system(request.messages)
        messages = self._translate_messages(remaining)
        system, messages = self._inject_cache_control(system, messages)

        kwargs: dict[str, Any] = {
            "model": request.model,
            "messages": messages,
            "max_tokens": request.max_tokens or 8192,
        }
        if system:
            kwargs["system"] = system
        tools = self._translate_tools(request.tools)
        if tools:
            kwargs["tools"] = tools
        if request.temperature is not None:
            kwargs["temperature"] = request.temperature
        if request.stop_sequences:
            kwargs["stop_sequences"] = request.stop_sequences

        # Handle provider-specific options
        extra_headers: dict[str, str] = {}
        if request.provider_options and "anthropic" in request.provider_options:
            opts = request.provider_options["anthropic"]
            if "thinking" in opts:
                kwargs["thinking"] = opts["thinking"]
            if "beta_headers" in opts:
                extra_headers["anthropic-beta"] = ",".join(opts["beta_headers"])
        if extra_headers:
            kwargs["extra_headers"] = extra_headers

        raw = await client.messages.create(**kwargs)
        return self._parse_response(raw)

    async def stream(self, request: Request) -> AsyncIterator[StreamEvent]:
        client = self._get_client()
        system, remaining = self._extract_system(request.messages)
        messages = self._translate_messages(remaining)
        system, messages = self._inject_cache_control(system, messages)

        kwargs: dict[str, Any] = {
            "model": request.model,
            "messages": messages,
            "max_tokens": request.max_tokens or 8192,
        }
        if system:
            kwargs["system"] = system
        tools = self._translate_tools(request.tools)
        if tools:
            kwargs["tools"] = tools
        if request.temperature is not None:
            kwargs["temperature"] = request.temperature

        async with client.messages.stream(**kwargs) as stream:
            msg_id = ""
            async for event in stream:
                event_type = event.type
                if event_type == "message_start":
                    msg_id = event.message.id
                    yield StreamEvent.message_start(
                        id=msg_id,
                        model=event.message.model,
                    )
                elif event_type == "content_block_start":
                    block = event.content_block
                    if block.type == "tool_use":
                        yield StreamEvent.tool_call_start(
                            id=block.id,
                            name=block.name,
                            index=event.index,
                        )
                elif event_type == "content_block_delta":
                    delta = event.delta
                    if delta.type == "text_delta":
                        yield StreamEvent.content_delta(delta.text, event.index)
                    elif delta.type == "input_json_delta":
                        yield StreamEvent.tool_call_delta(
                            delta.partial_json, event.index,
                        )
                    elif delta.type == "thinking_delta":
                        yield StreamEvent.thinking_delta(delta.thinking, event.index)
                elif event_type == "message_delta":
                    stop_map = {
                        "end_turn": FinishReason.STOP,
                        "tool_use": FinishReason.TOOL_USE,
                        "max_tokens": FinishReason.MAX_TOKENS,
                    }
                    fr = stop_map.get(event.delta.stop_reason, FinishReason.STOP)
                    usage = Usage(
                        output_tokens=getattr(event.usage, "output_tokens", 0),
                    )
                    yield StreamEvent.message_end(fr, usage)

    async def close(self) -> None:
        if self._client:
            await self._client.close()
            self._client = None
