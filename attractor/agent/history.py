"""Conversation history management and truncation."""

from __future__ import annotations

from enum import Enum
from typing import Any

from ..llm.models import ContentKind, Message, Role


class TruncationStrategy(str, Enum):
    NONE = "none"
    SLIDING_WINDOW = "sliding_window"
    HEAD_TAIL = "head_tail"


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

        if len(head) + len(tail) < len(non_system):
            summary = Message.system(
                f"[{len(non_system) - len(head) - len(tail)} messages truncated]"
            )
            self._messages = system_msgs + head + [summary] + tail
            return True
        return False
