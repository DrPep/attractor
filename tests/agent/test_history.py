"""Tests for conversation history."""

from attractor.agent.history import (
    ConversationHistory, TruncationStrategy, estimate_tokens,
)
from attractor.llm.models import Message


def test_history_add():
    h = ConversationHistory()
    h.add(Message.user("hello"))
    assert len(h.messages) == 1


def test_history_token_estimate():
    h = ConversationHistory()
    h.add(Message.user("x" * 400))  # ~100 tokens
    assert h.token_estimate > 0


def test_truncation_none():
    h = ConversationHistory([Message.user("x" * 4000)])
    result = h.truncate(TruncationStrategy.NONE, max_tokens=10)
    assert not result


def test_sliding_window_truncation():
    h = ConversationHistory()
    h.add(Message.system("system prompt"))
    for i in range(50):
        h.add(Message.user(f"message {i} " + "x" * 200))
    original_len = len(h.messages)
    result = h.truncate(TruncationStrategy.SLIDING_WINDOW, max_tokens=500)
    assert result
    assert len(h.messages) < original_len
    # System message preserved
    assert h.messages[0].role.value == "system"


def test_head_tail_truncation():
    h = ConversationHistory()
    h.add(Message.system("sys"))
    for i in range(20):
        h.add(Message.user(f"msg {i} " + "x" * 200))
    result = h.truncate(TruncationStrategy.HEAD_TAIL, max_tokens=500)
    assert result
    # Should have system + head + summary + tail
    assert len(h.messages) < 22
