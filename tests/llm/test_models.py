"""Tests for LLM data models."""

from attractor.llm.models import (
    ContentKind, ContentPart, FinishReason, Message, Request, Response,
    Role, ToolCallData, ToolChoice, ToolDefinition, Usage,
)


def test_message_system():
    msg = Message.system("You are helpful.")
    assert msg.role == Role.SYSTEM
    assert msg.text == "You are helpful."


def test_message_user():
    msg = Message.user("Hello")
    assert msg.role == Role.USER
    assert msg.text == "Hello"


def test_message_assistant():
    msg = Message.assistant("Hi there")
    assert msg.role == Role.ASSISTANT
    assert msg.text == "Hi there"


def test_message_tool_result():
    msg = Message.tool_result("call_123", "result data", is_error=False)
    assert msg.role == Role.TOOL
    assert msg.tool_call_id == "call_123"


def test_message_text_property():
    msg = Message(
        role=Role.ASSISTANT,
        content=[
            ContentPart.text_part("Hello "),
            ContentPart.text_part("world"),
        ],
    )
    assert msg.text == "Hello world"


def test_message_tool_calls_property():
    msg = Message(
        role=Role.ASSISTANT,
        content=[
            ContentPart.text_part("Let me check"),
            ContentPart.tool_call_part("call_1", "read_file", {"path": "test.py"}),
            ContentPart.tool_call_part("call_2", "shell", {"command": "ls"}),
        ],
    )
    assert len(msg.tool_calls) == 2
    assert msg.tool_calls[0].name == "read_file"
    assert msg.tool_calls[1].name == "shell"


def test_content_part_text():
    part = ContentPart.text_part("hello")
    assert part.kind == ContentKind.TEXT
    assert part.text == "hello"


def test_content_part_tool_call():
    part = ContentPart.tool_call_part("id1", "my_tool", {"arg": "val"})
    assert part.kind == ContentKind.TOOL_CALL
    assert part.tool_call.name == "my_tool"
    assert part.tool_call.arguments == {"arg": "val"}


def test_content_part_thinking():
    part = ContentPart.thinking_part("reasoning", signature="sig123")
    assert part.kind == ContentKind.THINKING
    assert part.thinking.text == "reasoning"
    assert part.thinking.signature == "sig123"
    assert not part.thinking.redacted


def test_content_part_redacted_thinking():
    part = ContentPart.thinking_part("opaque", redacted=True)
    assert part.kind == ContentKind.REDACTED_THINKING
    assert part.thinking.redacted


def test_usage_total_tokens():
    usage = Usage(input_tokens=100, output_tokens=50)
    assert usage.total_tokens == 150


def test_tool_choice_modes():
    assert ToolChoice.auto().mode == "auto"
    assert ToolChoice.none().mode == "none"
    assert ToolChoice.required().mode == "required"
    tc = ToolChoice.tool("my_tool")
    assert tc.mode == "tool"
    assert tc.name == "my_tool"


def test_request_creation():
    req = Request(
        model="claude-sonnet-4-5",
        messages=[Message.user("hello")],
        temperature=0.7,
    )
    assert req.model == "claude-sonnet-4-5"
    assert len(req.messages) == 1
    assert req.temperature == 0.7


def test_response_text():
    resp = Response(
        id="resp_1",
        model="test",
        provider="test",
        message=Message.assistant("The answer is 42"),
        finish_reason=FinishReason.STOP,
    )
    assert resp.text == "The answer is 42"
    assert resp.tool_calls == []


def test_tool_definition():
    td = ToolDefinition(
        name="read_file",
        description="Read a file",
        parameters={"type": "object", "properties": {"path": {"type": "string"}}},
    )
    assert td.name == "read_file"
