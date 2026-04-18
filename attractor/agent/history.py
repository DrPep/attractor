"""Conversation history management and truncation."""

from __future__ import annotations

from enum import Enum
from typing import Any

from ..llm.models import ContentKind, Message, Role


class TruncationStrategy(str, Enum):
    NONE = "none"
    SLIDING_WINDOW = "sliding_window"
    HEAD_TAIL = "head_tail"


def _tool_call_ids(msg: Message) -> set[str]:
    return {
        p.tool_call.id
        for p in msg.content
        if p.kind == ContentKind.TOOL_CALL and p.tool_call
    }


def _tool_result_ids(msg: Message) -> set[str]:
    return {
        p.tool_result.tool_call_id
        for p in msg.content
        if p.kind == ContentKind.TOOL_RESULT and p.tool_result
    }


def heal_orphan_tool_pairs(messages: list[Message]) -> list[Message]:
    """Drop messages that would leave tool_use/tool_result pairs broken.

    Anthropic (and others) reject requests where a tool_result appears without
    the matching tool_use in the preceding assistant turn. Truncation can
    produce exactly that: the kept window starts with a user message whose
    first block is a tool_result referencing a tool_use we dropped.

    Heals two orphan cases, iterating to a fixed point:
      • tool_result blocks whose tool_call_id was never produced earlier in
        the kept sequence (dropped);
      • tool_use blocks not followed by matching tool_results later in the
        kept sequence, except when the assistant message is the very last
        one (that represents an in-flight turn, which is legal).
    """
    current = list(messages)
    while True:
        n = len(current)
        produced_before: list[set[str]] = [set()] * n
        seen: set[str] = set()
        for i, m in enumerate(current):
            produced_before[i] = set(seen)
            seen |= _tool_call_ids(m)

        results_after: list[set[str]] = [set()] * n
        seen2: set[str] = set()
        for i in range(n - 1, -1, -1):
            results_after[i] = set(seen2)
            seen2 |= _tool_result_ids(current[i])

        dropped = False
        out: list[Message] = []
        for i, m in enumerate(current):
            r_ids = _tool_result_ids(m)
            if r_ids and not r_ids.issubset(produced_before[i]):
                dropped = True
                continue
            c_ids = _tool_call_ids(m)
            is_last = i == n - 1
            if c_ids and not is_last and not c_ids.issubset(results_after[i]):
                dropped = True
                continue
            out.append(m)
        if not dropped:
            return out
        current = out


def estimate_tokens(messages: list[Message]) -> int:
    """Rough token estimate: ~4 chars per token."""
    total = 0
    for msg in messages:
        for part in msg.content:
            if part.kind == ContentKind.TEXT and part.text:
                total += len(part.text) // 4
            elif part.kind == ContentKind.TOOL_CALL and part.tool_call:
                import json
                args = part.tool_call.arguments
                if isinstance(args, dict):
                    args = json.dumps(args)
                total += (len(part.tool_call.name) + len(str(args))) // 4
            elif part.kind == ContentKind.TOOL_RESULT and part.tool_result:
                content = part.tool_result.content
                total += len(str(content)) // 4
    return total


class ConversationHistory:
    """Manages conversation history with truncation support."""

    def __init__(self, messages: list[Message] | None = None):
        self._messages: list[Message] = messages or []

    @property
    def messages(self) -> list[Message]:
        return self._messages

    def add(self, message: Message) -> None:
        self._messages.append(message)

    def extend(self, messages: list[Message]) -> None:
        self._messages.extend(messages)

    def clear(self) -> None:
        self._messages.clear()

    @property
    def token_estimate(self) -> int:
        return estimate_tokens(self._messages)

    def truncate(
        self,
        strategy: TruncationStrategy = TruncationStrategy.SLIDING_WINDOW,
        max_tokens: int = 100000,
    ) -> bool:
        """Truncate history if over max_tokens. Returns True if truncated."""
        if strategy == TruncationStrategy.NONE:
            return False

        current = self.token_estimate
        if current <= max_tokens:
            return False

        if strategy == TruncationStrategy.SLIDING_WINDOW:
            return self._truncate_sliding_window(max_tokens)
        elif strategy == TruncationStrategy.HEAD_TAIL:
            return self._truncate_head_tail(max_tokens)
        return False

    def _truncate_sliding_window(self, max_tokens: int) -> bool:
        # Keep system messages + most recent messages
        system_msgs = [m for m in self._messages if m.role == Role.SYSTEM]
        non_system = [m for m in self._messages if m.role != Role.SYSTEM]

        system_tokens = estimate_tokens(system_msgs)
        budget = max_tokens - system_tokens

        kept: list[Message] = []
        running = 0
        for msg in reversed(non_system):
            msg_tokens = estimate_tokens([msg])
            if running + msg_tokens > budget:
                break
            kept.insert(0, msg)
            running += msg_tokens

        # Always keep at least the most recent non-system message so the
        # request to the LLM is never empty after system extraction.
        if not kept and non_system:
            kept = [non_system[-1]]

        kept = heal_orphan_tool_pairs(kept)

        if len(kept) < len(non_system):
            summary = Message.system(
                "[Earlier conversation truncated to fit context window]"
            )
            self._messages = system_msgs + [summary] + kept
            return True
        return False

    def _truncate_head_tail(self, max_tokens: int) -> bool:
        system_msgs = [m for m in self._messages if m.role == Role.SYSTEM]
        non_system = [m for m in self._messages if m.role != Role.SYSTEM]

        if len(non_system) <= 4:
            return False

        system_tokens = estimate_tokens(system_msgs)
        budget = max_tokens - system_tokens - 50  # reserve for summary

        # Keep first 2 and last N messages
        head = non_system[:2]
        head_tokens = estimate_tokens(head)
        tail_budget = budget - head_tokens

        tail: list[Message] = []
        running = 0
        for msg in reversed(non_system[2:]):
            msg_tokens = estimate_tokens([msg])
            if running + msg_tokens > tail_budget:
                break
            tail.insert(0, msg)
            running += msg_tokens

        # Healing runs over the concatenation of head+tail, since the summary
        # we'll insert between them is a SYSTEM message that some adapters pull
        # out of the conversation — we can't rely on it to shield a broken pair.
        combined = heal_orphan_tool_pairs(head + tail)
        if len(combined) < len(non_system):
            summary = Message.system(
                f"[{len(non_system) - len(combined)} messages truncated]"
            )
            self._messages = system_msgs + [summary] + combined
            return True
        return False
