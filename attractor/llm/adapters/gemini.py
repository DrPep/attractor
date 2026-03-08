"""Google Gemini adapter."""

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


class GeminiAdapter(ProviderAdapter):
    """Adapter for Google Gemini API."""

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
        return "gemini"

    def _get_client(self) -> Any:
        if self._client is None:
            from google import genai
            self._client = genai.Client(api_key=self._api_key)
        return self._client

    def _extract_system(self, messages: list[Message]) -> tuple[str, list[Message]]:
        system_parts: list[str] = []
        remaining: list[Message] = []
        for msg in messages:
            if msg.role in (Role.SYSTEM, Role.DEVELOPER):
                system_parts.append(msg.text)
            else:
                remaining.append(msg)
        return "\n\n".join(system_parts), remaining

    def _translate_messages(self, messages: list[Message]) -> list[dict[str, Any]]:
        from google.genai import types
        result = []
        for msg in messages:
            role = "user" if msg.role in (Role.USER, Role.TOOL) else "model"
            parts = []
            for p in msg.content:
                if p.kind == ContentKind.TEXT:
                    parts.append(types.Part(text=p.text or ""))
                elif p.kind == ContentKind.IMAGE and p.image:
                    if p.image.data:
                        parts.append(types.Part(
                            inline_data=types.Blob(
                                mime_type=p.image.media_type or "image/png",
                                data=p.image.data,
                            )
                        ))
                    elif p.image.url:
                        parts.append(types.Part(
                            file_data=types.FileData(file_uri=p.image.url)
                        ))
                elif p.kind == ContentKind.TOOL_CALL and p.tool_call:
                    args = p.tool_call.arguments
                    if isinstance(args, str):
                        try:
                            args = json.loads(args)
                        except json.JSONDecodeError:
                            args = {"raw": args}
                    parts.append(types.Part(
                        function_call=types.FunctionCall(
                            name=p.tool_call.name,
                            args=args,
                        )
                    ))
                elif p.kind == ContentKind.TOOL_RESULT and p.tool_result:
                    content = p.tool_result.content
                    if isinstance(content, str):
                        output = {"result": content}
                    else:
                        output = content
                    parts.append(types.Part(
                        function_response=types.FunctionResponse(
                            name=p.tool_result.tool_call_id,
                            response=output,
                        )
                    ))
            if parts:
                result.append(types.Content(role=role, parts=parts))
        return result

    def _translate_tools(
        self, tools: list[ToolDefinition] | None,
    ) -> list[Any] | None:
        if not tools:
            return None
        from google.genai import types
        declarations = []
        for t in tools:
            declarations.append(types.FunctionDeclaration(
                name=t.name,
                description=t.description,
                parameters=t.parameters if t.parameters else None,
            ))
        return [types.Tool(function_declarations=declarations)]

    def _parse_response(self, raw: Any, model: str) -> Response:
        content_parts: list[ContentPart] = []
        finish_reason = FinishReason.STOP

        for candidate in raw.candidates:
            for part in candidate.content.parts:
                if part.text:
                    content_parts.append(ContentPart.text_part(part.text))
                elif part.function_call:
                    call_id = f"call_{uuid.uuid4().hex[:12]}"
                    content_parts.append(ContentPart.tool_call_part(
                        id=call_id,
                        name=part.function_call.name,
                        arguments=dict(part.function_call.args) if part.function_call.args else {},
                    ))
                    finish_reason = FinishReason.TOOL_USE

            fr_map = {"STOP": FinishReason.STOP, "MAX_TOKENS": FinishReason.MAX_TOKENS,
                       "SAFETY": FinishReason.CONTENT_FILTER}
            raw_reason = str(getattr(candidate, "finish_reason", "STOP"))
            finish_reason = fr_map.get(raw_reason, finish_reason)

        usage = Usage()
        if hasattr(raw, "usage_metadata") and raw.usage_metadata:
            um = raw.usage_metadata
            usage = Usage(
                input_tokens=getattr(um, "prompt_token_count", 0) or 0,
                output_tokens=getattr(um, "candidates_token_count", 0) or 0,
                cache_read_tokens=getattr(um, "cached_content_token_count", 0) or 0,
            )

        return Response(
            id=getattr(raw, "response_id", "") or "",
            model=model,
            provider="gemini",
            message=Message(role=Role.ASSISTANT, content=content_parts),
            finish_reason=finish_reason,
            usage=usage,
        )

    async def complete(self, request: Request) -> Response:
        from google.genai import types
        client = self._get_client()
        system_instruction, remaining = self._extract_system(request.messages)
        contents = self._translate_messages(remaining)

        config_kwargs: dict[str, Any] = {}
        if request.temperature is not None:
            config_kwargs["temperature"] = request.temperature
        if request.max_tokens is not None:
            config_kwargs["max_output_tokens"] = request.max_tokens
        if request.stop_sequences:
            config_kwargs["stop_sequences"] = request.stop_sequences

        tools = self._translate_tools(request.tools)

        kwargs: dict[str, Any] = {
            "model": request.model,
            "contents": contents,
        }
        if system_instruction:
            config_kwargs["system_instruction"] = system_instruction
        if tools:
            kwargs["config"] = types.GenerateContentConfig(
                tools=tools, **config_kwargs,
            )
        elif config_kwargs:
            kwargs["config"] = types.GenerateContentConfig(**config_kwargs)

        raw = await client.aio.models.generate_content(**kwargs)
        return self._parse_response(raw, request.model)

    async def stream(self, request: Request) -> AsyncIterator[StreamEvent]:
        from google.genai import types
        client = self._get_client()
        system_instruction, remaining = self._extract_system(request.messages)
        contents = self._translate_messages(remaining)

        config_kwargs: dict[str, Any] = {}
        if request.temperature is not None:
            config_kwargs["temperature"] = request.temperature
        if request.max_tokens is not None:
            config_kwargs["max_output_tokens"] = request.max_tokens
        if system_instruction:
            config_kwargs["system_instruction"] = system_instruction

        tools = self._translate_tools(request.tools)
        if tools:
            config_kwargs["tools"] = tools

        kwargs: dict[str, Any] = {
            "model": request.model,
            "contents": contents,
        }
        if config_kwargs:
            kwargs["config"] = types.GenerateContentConfig(**config_kwargs)

        yielded_start = False
        async for chunk in await client.aio.models.generate_content_stream(**kwargs):
            if not yielded_start:
                yield StreamEvent.message_start(id="", model=request.model)
                yielded_start = True
            for part in chunk.candidates[0].content.parts:
                if part.text:
                    yield StreamEvent.content_delta(part.text)
                elif part.function_call:
                    call_id = f"call_{uuid.uuid4().hex[:12]}"
                    yield StreamEvent.tool_call_start(call_id, part.function_call.name)
                    args = json.dumps(dict(part.function_call.args)) if part.function_call.args else ""
                    if args:
                        yield StreamEvent.tool_call_delta(args)

        usage = Usage()
        if hasattr(chunk, "usage_metadata") and chunk.usage_metadata:
            um = chunk.usage_metadata
            usage = Usage(
                input_tokens=getattr(um, "prompt_token_count", 0) or 0,
                output_tokens=getattr(um, "candidates_token_count", 0) or 0,
            )
        yield StreamEvent.message_end(FinishReason.STOP, usage)

    async def close(self) -> None:
        self._client = None
