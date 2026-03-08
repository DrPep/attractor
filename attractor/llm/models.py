"""Core data models for the Unified LLM Client."""

from __future__ import annotations

from datetime import datetime
from enum import Enum
from typing import Any

from pydantic import BaseModel, Field


class Role(str, Enum):
    SYSTEM = "system"
    USER = "user"
    ASSISTANT = "assistant"
    TOOL = "tool"
    DEVELOPER = "developer"


class ContentKind(str, Enum):
    TEXT = "text"
    IMAGE = "image"
    AUDIO = "audio"
    DOCUMENT = "document"
    TOOL_CALL = "tool_call"
    TOOL_RESULT = "tool_result"
    THINKING = "thinking"
    REDACTED_THINKING = "redacted_thinking"


class ImageData(BaseModel):
    url: str | None = None
    data: bytes | None = None
    media_type: str | None = None
    detail: str | None = None


class AudioData(BaseModel):
    url: str | None = None
    data: bytes | None = None
    media_type: str | None = None


class DocumentData(BaseModel):
    url: str | None = None
    data: bytes | None = None
    media_type: str | None = None
    file_name: str | None = None


class ToolCallData(BaseModel):
    id: str
    name: str
    arguments: dict[str, Any] | str = Field(default_factory=dict)
    type: str = "function"


class ToolResultData(BaseModel):
    tool_call_id: str
    content: str | dict[str, Any] = ""
    is_error: bool = False
    image_data: bytes | None = None
    image_media_type: str | None = None


class ThinkingData(BaseModel):
    text: str = ""
    signature: str | None = None
    redacted: bool = False


class ContentPart(BaseModel):
    kind: ContentKind | str
    text: str | None = None
    image: ImageData | None = None
    audio: AudioData | None = None
    document: DocumentData | None = None
    tool_call: ToolCallData | None = None
    tool_result: ToolResultData | None = None
    thinking: ThinkingData | None = None

    @classmethod
    def text_part(cls, text: str) -> ContentPart:
        return cls(kind=ContentKind.TEXT, text=text)

    @classmethod
    def image_part(
        cls, url: str | None = None, data: bytes | None = None,
        media_type: str | None = None, detail: str | None = None,
    ) -> ContentPart:
        return cls(kind=ContentKind.IMAGE, image=ImageData(
            url=url, data=data, media_type=media_type, detail=detail,
        ))

    @classmethod
    def tool_call_part(
        cls, id: str, name: str, arguments: dict[str, Any] | str,
    ) -> ContentPart:
        return cls(kind=ContentKind.TOOL_CALL, tool_call=ToolCallData(
            id=id, name=name, arguments=arguments,
        ))

    @classmethod
    def tool_result_part(
        cls, tool_call_id: str, content: str | dict[str, Any],
        is_error: bool = False,
    ) -> ContentPart:
        return cls(kind=ContentKind.TOOL_RESULT, tool_result=ToolResultData(
            tool_call_id=tool_call_id, content=content, is_error=is_error,
        ))

    @classmethod
    def thinking_part(
        cls, text: str, signature: str | None = None, redacted: bool = False,
    ) -> ContentPart:
        kind = ContentKind.REDACTED_THINKING if redacted else ContentKind.THINKING
        return cls(kind=kind, thinking=ThinkingData(
            text=text, signature=signature, redacted=redacted,
        ))


class Message(BaseModel):
    role: Role
    content: list[ContentPart]
    name: str | None = None
    tool_call_id: str | None = None

    @property
    def text(self) -> str:
        return "".join(
            p.text for p in self.content
            if p.kind == ContentKind.TEXT and p.text
        )

    @property
    def tool_calls(self) -> list[ToolCallData]:
        return [
            p.tool_call for p in self.content
            if p.kind == ContentKind.TOOL_CALL and p.tool_call
        ]

    @classmethod
    def system(cls, text: str) -> Message:
        return cls(role=Role.SYSTEM, content=[ContentPart.text_part(text)])

    @classmethod
    def user(cls, text: str) -> Message:
        return cls(role=Role.USER, content=[ContentPart.text_part(text)])

    @classmethod
    def assistant(cls, text: str) -> Message:
        return cls(role=Role.ASSISTANT, content=[ContentPart.text_part(text)])

    @classmethod
    def tool_result(
        cls, tool_call_id: str, content: str, is_error: bool = False,
    ) -> Message:
        return cls(
            role=Role.TOOL,
            content=[ContentPart.tool_result_part(tool_call_id, content, is_error)],
            tool_call_id=tool_call_id,
        )


class ToolDefinition(BaseModel):
    name: str
    description: str = ""
    parameters: dict[str, Any] = Field(default_factory=dict)


class ToolChoice(BaseModel):
    mode: str = "auto"  # auto, none, required, or specific tool name
    name: str | None = None

    @classmethod
    def auto(cls) -> ToolChoice:
        return cls(mode="auto")

    @classmethod
    def none(cls) -> ToolChoice:
        return cls(mode="none")

    @classmethod
    def required(cls) -> ToolChoice:
        return cls(mode="required")

    @classmethod
    def tool(cls, name: str) -> ToolChoice:
        return cls(mode="tool", name=name)


class ResponseFormat(BaseModel):
    type: str = "text"  # text, json, json_schema
    json_schema: dict[str, Any] | None = None


class FinishReason(str, Enum):
    STOP = "stop"
    TOOL_USE = "tool_use"
    MAX_TOKENS = "max_tokens"
    CONTENT_FILTER = "content_filter"
    ERROR = "error"


class Usage(BaseModel):
    input_tokens: int = 0
    output_tokens: int = 0
    cache_read_tokens: int = 0
    cache_write_tokens: int = 0

    @property
    def total_tokens(self) -> int:
        return self.input_tokens + self.output_tokens


class RateLimitInfo(BaseModel):
    limit: int | None = None
    remaining: int | None = None
    reset_at: datetime | None = None


class Warning(BaseModel):
    code: str
    message: str


class Request(BaseModel):
    model: str
    messages: list[Message]
    provider: str | None = None
    tools: list[ToolDefinition] | None = None
    tool_choice: ToolChoice | None = None
    response_format: ResponseFormat | None = None
    temperature: float | None = None
    top_p: float | None = None
    max_tokens: int | None = None
    stop_sequences: list[str] | None = None
    reasoning_effort: str | None = None
    metadata: dict[str, str] | None = None
    provider_options: dict[str, Any] | None = None


class Response(BaseModel):
    id: str = ""
    model: str = ""
    provider: str = ""
    message: Message = Field(default_factory=lambda: Message(role=Role.ASSISTANT, content=[]))
    finish_reason: FinishReason = FinishReason.STOP
    usage: Usage = Field(default_factory=Usage)
    raw: dict[str, Any] | None = None
    warnings: list[Warning] = Field(default_factory=list)
    rate_limit: RateLimitInfo | None = None

    @property
    def text(self) -> str:
        return self.message.text

    @property
    def tool_calls(self) -> list[ToolCallData]:
        return self.message.tool_calls
