"""Tests for conversation history."""

from attractor.agent.history import (
    ConversationHistory, TruncationStrategy, estimate_tokens,
    heal_orphan_tool_pairs,
)
from attractor.llm.models import ContentPart, Message, Role


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


# ── tool-pair healing ────────────────────────────────────────────────────

def _assistant_with_tool_call(text: str, tid: str, tool: str = "shell") -> Message:
    return Message(role=Role.ASSISTANT, content=[
        ContentPart.text_part(text),
        ContentPart.tool_call_part(id=tid, name=tool, arguments={}),
    ])


def _tool_result_msg(tid: str, body: str = "ok") -> Message:
    return Message.tool_result(tool_call_id=tid, content=body)


def test_heal_passthrough_when_no_tools():
    msgs = [Message.user("a"), Message.assistant("b"), Message.user("c")]
    assert heal_orphan_tool_pairs(msgs) == msgs


def test_heal_drops_leading_orphan_tool_result():
    # tool_result at head with no earlier tool_call → drop it.
    msgs = [
        _tool_result_msg("toolu_lost"),
        Message.user("follow-up"),
        _assistant_with_tool_call("ok", "toolu_new"),
        _tool_result_msg("toolu_new"),
    ]
    out = heal_orphan_tool_pairs(msgs)
    assert out == msgs[1:]


def test_heal_drops_multiple_leading_orphans():
    msgs = [
        _tool_result_msg("a"),
        _tool_result_msg("b"),
        Message.user("text"),
    ]
    assert heal_orphan_tool_pairs(msgs) == [msgs[-1]]


def test_heal_keeps_intact_pair():
    msgs = [
        _assistant_with_tool_call("call", "t1"),
        _tool_result_msg("t1"),
        Message.assistant("done"),
    ]
    assert heal_orphan_tool_pairs(msgs) == msgs


def test_heal_keeps_trailing_unanswered_tool_use():
    # Assistant ends with a tool_use as the last message — this is an
    # in-flight turn and must be preserved.
    msgs = [
        Message.user("start"),
        _assistant_with_tool_call("working", "t1"),
    ]
    assert heal_orphan_tool_pairs(msgs) == msgs


def test_heal_drops_mid_sequence_unanswered_tool_use():
    # Assistant tool_use with no matching result before the next turn → drop it.
    msgs = [
        Message.user("start"),
        _assistant_with_tool_call("mid", "t_missing"),
        Message.user("keep going"),
        _assistant_with_tool_call("finish", "t_ok"),
        _tool_result_msg("t_ok"),
    ]
    out = heal_orphan_tool_pairs(msgs)
    assert all(
        not any(p.tool_call and p.tool_call.id == "t_missing" for p in m.content)
        for m in out
    )
    # The good pair must survive.
    assert out[-2:] == msgs[-2:]


def test_sliding_window_does_not_produce_orphan_tool_result():
    # Regression: long history with several tool rounds. Truncation should
    # never leave a tool_result at the head with no preceding tool_use.
    h = ConversationHistory()
    h.add(Message.system("sys"))
    for i in range(20):
        h.add(_assistant_with_tool_call("x" * 200, f"t{i}"))
        h.add(_tool_result_msg(f"t{i}", "r" * 200))
    h.truncate(TruncationStrategy.SLIDING_WINDOW, max_tokens=600)
    non_system = [m for m in h.messages if m.role != Role.SYSTEM]
    # First non-system message must not be a bare tool_result.
    first = non_system[0]
    assert not any(p.tool_result for p in first.content), (
        "sliding-window left an orphan tool_result at the head"
    )


def test_head_tail_does_not_split_tool_pair():
    h = ConversationHistory()
    h.add(Message.system("sys"))
    h.add(Message.user("kickoff"))
    for i in range(15):
        h.add(_assistant_with_tool_call("x" * 200, f"t{i}"))
        h.add(_tool_result_msg(f"t{i}", "r" * 200))
    h.truncate(TruncationStrategy.HEAD_TAIL, max_tokens=700)
    # No orphan tool_result anywhere in the kept non-system sequence.
    produced: set[str] = set()
    for m in h.messages:
        if m.role == Role.SYSTEM:
            continue
        for p in m.content:
            if p.tool_call:
                produced.add(p.tool_call.id)
            if p.tool_result:
                assert p.tool_result.tool_call_id in produced, (
                    "head-tail left a split tool pair"
                )
