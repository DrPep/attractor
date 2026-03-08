"""Tests for streaming types and accumulator."""

from attractor.llm.streaming import (
    StreamAccumulator, StreamEvent, StreamEventType,
)
from attractor.llm.models import FinishReason, Usage


def test_stream_event_content_delta():
    event = StreamEvent.content_delta("hello", index=0)
    assert event.type == StreamEventType.CONTENT_DELTA
    assert event.text == "hello"


def test_stream_event_tool_call():
    event = StreamEvent.tool_call_start("call_1", "my_tool")
    assert event.type == StreamEventType.TOOL_CALL_START
    assert event.tool_call_id == "call_1"


def test_accumulator_text():
    acc = StreamAccumulator()
    acc.process(StreamEvent.message_start("msg_1", "test-model"))
    acc.process(StreamEvent.content_delta("Hello ", 0))
    acc.process(StreamEvent.content_delta("world", 0))
    acc.process(StreamEvent.message_end(FinishReason.STOP, Usage(input_tokens=10, output_tokens=5)))

    resp = acc.to_response()
    assert resp.text == "Hello world"
    assert resp.id == "msg_1"
    assert resp.finish_reason == FinishReason.STOP
    assert resp.usage.total_tokens == 15


def test_accumulator_tool_calls():
    acc = StreamAccumulator()
    acc.process(StreamEvent.message_start("msg_2", "test"))
    acc.process(StreamEvent.tool_call_start("call_1", "read_file", 0))
    acc.process(StreamEvent.tool_call_delta('{"path": "test.py"}', 0))
    acc.process(StreamEvent.message_end(FinishReason.TOOL_USE))

    resp = acc.to_response()
    assert len(resp.tool_calls) == 1
    assert resp.tool_calls[0].name == "read_file"
    assert resp.tool_calls[0].arguments == {"path": "test.py"}
    assert resp.finish_reason == FinishReason.TOOL_USE
