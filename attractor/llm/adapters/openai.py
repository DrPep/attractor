"""OpenAI adapter using the Responses API."""

from __future__ import annotations

import json
import uuid
from typing import Any, AsyncIterator

from ..adapter import ProviderAdapter
from ..models import (
    ContentKind, ContentPart, FinishReason, Message, Request, Response,
    Role, ToolCallData, ToolDefinition, Usage,
)
from ..streaming import StreamEvent


class OpenAIAdapter(ProviderAdapter):
    """Adapter for OpenAI using the Responses API (/v1/responses)."""

    def __init__(
        self,
        api_key: str,
        base_url: str | None = None,
        org_id: str | None = None,
    ):
        self._api_key = api_key
        self._base_url = base_url
        self._org_id = org_id
        self._client: Any = None

    @property
    def provider_name(self) -> str:
        return "openai"

    def _get_client(self) -> Any:
        if self._client is None:
            from openai import AsyncOpenAI
            kwargs: dict[str, Any] = {"api_key": self._api_key}
            if self._base_url:
                kwargs["base_url"] = self._base_url
            if self._org_id:
                kwargs["organization"] = self._org_id
            self._client = AsyncOpenAI(**kwargs)
        return self._client

    def _translate_messages(self, messages: list[Message]) -> list[dict[str, Any]]:
        result: list[dict[str, Any]] = []
        for msg in messages:
            if msg.role == Role.SYSTEM:
                result.append({
                    "role": "system",
                    "content": msg.text,
                })
            elif msg.role == Role.DEVELOPER:
                result.append({
                    "role": "developer",
                    "content": msg.text,
                })
            elif msg.role == Role.USER:
                parts = []
                for p in msg.content:
                    if p.kind == ContentKind.TEXT:
                        parts.append({"type": "text", "text": p.text or ""})
                    elif p.kind == ContentKind.IMAGE and p.image:
                        if p.image.url:
                            parts.append({
                                "type": "image_url",
                                "image_url": {"url": p.image.url},
                            })
                        elif p.image.data:
                            import base64
                            mt = p.image.media_type or "image/png"
                            b64 = base64.b64encode(p.image.data).decode()
                            parts.append({
                                "type": "image_url",
                                "image_url": {"url": f"data:{mt};base64,{b64}"},
                            })
                if len(parts) == 1 and parts[0].get("type") == "text":
                    result.append({"role": "user", "content": parts[0]["text"]})
                else:
                    result.append({"role": "user", "content": parts})
            elif msg.role == Role.ASSISTANT:
                content_parts: list[dict[str, Any]] = []
                for p in msg.content:
                    if p.kind == ContentKind.TEXT:
                        content_parts.append({"type": "text", "text": p.text or ""})
                    elif p.kind == ContentKind.TOOL_CALL and p.tool_call:
                        content_parts.append({
                            "type": "function_call",
                            "id": p.tool_call.id,
                            "name": p.tool_call.name,
                            "arguments": (
                                json.dumps(p.tool_call.arguments)
                                if isinstance(p.tool_call.arguments, dict)
                                else p.tool_call.arguments
                            ),
                        })
                if content_parts:
                    result.append({"role": "assistant", "content": content_parts})
            elif msg.role == Role.TOOL:
                for p in msg.content:
                    if p.kind == ContentKind.TOOL_RESULT and p.tool_result:
                        content = p.tool_result.content
                        if isinstance(content, dict):
                            content = json.dumps(content)
                        result.append({
                            "type": "function_call_output",
                            "call_id": p.tool_result.tool_call_id,
                            "output": content,
                        })
        return result

    def _translate_tools(
        self, tools: list[ToolDefinition] | None,
    ) -> list[dict[str, Any]] | None:
        if not tools:
            return None
        return [
            {
                "type": "function",
                "name": t.name,
                "description": t.description,
                "parameters": t.parameters,
            }
            for t in tools
        ]

    def _parse_response(self, raw_response: Any) -> Response:
        content_parts: list[ContentPart] = []
        finish_reason = FinishReason.STOP

        for item in raw_response.output:
            if item.type == "message":
                for part in item.content:
                    if part.type == "output_text":
                        content_parts.append(ContentPart.text_part(part.text))
                if item.status == "completed":
                    finish_reason = FinishReason.STOP
            elif item.type == "function_call":
                try:
                    args = json.loads(item.arguments) if item.arguments else {}
                except json.JSONDecodeError:
                    args = item.arguments
                content_parts.append(ContentPart.tool_call_part(
                    id=item.call_id or item.id,
                    name=item.name,
                    arguments=args,
                ))
                finish_reason = FinishReason.TOOL_USE
            elif item.type == "reasoning":
                for s in getattr(item, "summary", []) or []:
                    if hasattr(s, "text"):
                        content_parts.append(ContentPart.thinking_part(s.text))

        usage = Usage()
        if hasattr(raw_response, "usage") and raw_response.usage:
            u = raw_response.usage
            usage = Usage(
                input_tokens=getattr(u, "input_tokens", 0),
                output_tokens=getattr(u, "output_tokens", 0),
                cache_read_tokens=getattr(u, "input_tokens_details", None)
                    and getattr(u.input_tokens_details, "cached_tokens", 0) or 0,
            )

        return Response(
            id=raw_response.id,
            model=getattr(raw_response, "model", ""),
            provider="openai",
            message=Message(role=Role.ASSISTANT, content=content_parts),
            finish_reason=finish_reason,
            usage=usage,
        )

    async def complete(self, request: Request) -> Response:
        client = self._get_client()
        kwargs: dict[str, Any] = {
            "model": request.model,
            "input": self._translate_messages(request.messages),
        }
        tools = self._translate_tools(request.tools)
        if tools:
            kwargs["tools"] = tools
        if request.temperature is not None:
            kwargs["temperature"] = request.temperature
        if request.max_tokens is not None:
            kwargs["max_output_tokens"] = request.max_tokens
        if request.reasoning_effort and request.reasoning_effort != "none":
            kwargs["reasoning"] = {"effort": request.reasoning_effort}
        if request.provider_options and "openai" in request.provider_options:
            kwargs.update(request.provider_options["openai"])

        raw = await client.responses.create(**kwargs)
        return self._parse_response(raw)

    async def stream(self, request: Request) -> AsyncIterator[StreamEvent]:
        client = self._get_client()
        kwargs: dict[str, Any] = {
            "model": request.model,
            "input": self._translate_messages(request.messages),
            "stream": True,
        }
        tools = self._translate_tools(request.tools)
        if tools:
            kwargs["tools"] = tools
        if request.temperature is not None:
            kwargs["temperature"] = request.temperature
        if request.max_tokens is not None:
            kwargs["max_output_tokens"] = request.max_tokens

        async with client.responses.create(**kwargs) as stream:
            async for event in stream:
                event_type = getattr(event, "type", "")
                if event_type == "response.created":
                    yield StreamEvent.message_start(
                        id=event.response.id,
                        model=event.response.model or request.model,
                    )
                elif event_type == "response.output_text.delta":
                    yield StreamEvent.content_delta(event.delta)
                elif event_type == "response.function_call_arguments.delta":
                    yield StreamEvent.tool_call_delta(event.delta)
                elif event_type == "response.completed":
                    usage = Usage()
                    if hasattr(event.response, "usage") and event.response.usage:
                        u = event.response.usage
                        usage = Usage(
                            input_tokens=getattr(u, "input_tokens", 0),
                            output_tokens=getattr(u, "output_tokens", 0),
                        )
                    yield StreamEvent.message_end(FinishReason.STOP, usage)

    async def close(self) -> None:
        if self._client:
            await self._client.close()
            self._client = None
