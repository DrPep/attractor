"""Loop detection for the agent loop."""

from __future__ import annotations

import hashlib
import json
from collections import deque

from pydantic import BaseModel


class LoopDetection(BaseModel):
    is_looping: bool = False
    description: str = ""
    pattern_length: int = 0


class LoopDetector:
    """Detects when the agent is stuck in a loop calling the same tools."""

    def __init__(self, window_size: int = 20, threshold: int = 3):
        self._window: deque[str] = deque(maxlen=window_size)
        self._threshold = threshold

    def record(self, tool_name: str, args: dict) -> LoopDetection:
        """Record a tool call and check for loops."""
        # Create a hash of tool name + args
        sig = self._signature(tool_name, args)
        self._window.append(sig)

        # Check for repeating patterns
        window = list(self._window)
        n = len(window)

        # Check for same call repeated threshold times
        if n >= self._threshold:
            last = window[-1]
            consecutive = sum(
                1 for i in range(n - 1, -1, -1) if window[i] == last
            )
            if consecutive >= self._threshold:
                return LoopDetection(
                    is_looping=True,
                    description=f"Same tool call repeated {consecutive} times: {tool_name}",
                    pattern_length=1,
                )

        # Check for repeating sequences (length 2 and 3)
        for pattern_len in (2, 3):
            if n >= pattern_len * self._threshold:
                pattern = window[-pattern_len:]
                repeats = 0
                for i in range(self._threshold):
                    start = n - pattern_len * (i + 1)
                    if start < 0:
                        break
                    segment = window[start:start + pattern_len]
                    if segment == pattern:
                        repeats += 1
                if repeats >= self._threshold:
                    return LoopDetection(
                        is_looping=True,
                        description=f"Repeating pattern of {pattern_len} calls detected",
                        pattern_length=pattern_len,
                    )

        return LoopDetection()

    def reset(self) -> None:
        self._window.clear()

    @staticmethod
    def _signature(tool_name: str, args: dict) -> str:
        args_str = json.dumps(args, sort_keys=True, default=str)
        h = hashlib.md5(args_str.encode()).hexdigest()[:8]
        return f"{tool_name}:{h}"
