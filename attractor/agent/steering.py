"""Steering and follow-up message queues."""

from __future__ import annotations

import asyncio

from ..llm.models import Message


class SteeringManager:
    """Manages steering and follow-up message injection."""

    def __init__(self) -> None:
        self._steering: asyncio.Queue[str] = asyncio.Queue()
        self._followups: asyncio.Queue[str] = asyncio.Queue()

    def steer(self, message: str) -> None:
        """Add a message to inject between tool rounds."""
        self._steering.put_nowait(message)

    def follow_up(self, message: str) -> None:
        """Add a message to process after current turn completes."""
        self._followups.put_nowait(message)

    def drain_steering(self) -> list[Message]:
        """Get all pending steering messages as user Messages."""
        messages: list[Message] = []
        while not self._steering.empty():
            try:
                text = self._steering.get_nowait()
                messages.append(Message.user(text))
            except asyncio.QueueEmpty:
                break
        return messages

    def drain_followups(self) -> list[Message]:
        """Get all pending follow-up messages as user Messages."""
        messages: list[Message] = []
        while not self._followups.empty():
            try:
                text = self._followups.get_nowait()
                messages.append(Message.user(text))
            except asyncio.QueueEmpty:
                break
        return messages
