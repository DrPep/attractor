"""Progress tracking for pipeline execution."""

from __future__ import annotations

from typing import Any

from ..agent.events import Event, EventType
from .graph import Node

try:
    from rich.console import Console

    HAS_RICH = True
except ImportError:  # pragma: no cover
    HAS_RICH = False


class ProgressTracker:
    """Displays live progress during pipeline execution.

    Uses rich for a spinner + status line when available, falls back to plain print.
    """

    def __init__(self, total_nodes: int = 0) -> None:
        self._total = total_nodes
        self._nodes_done = 0
        self._current_node: str = ""
        self._status: Any | None = None  # rich.status.Status when active

        if HAS_RICH:
            self._console = Console(stderr=True)
        else:
            self._console = None

    # ── Runner callbacks ────────────────────────────────────────────────

    def on_node_start(self, node: Node, index: int, total: int) -> None:
        """Called when a node begins execution."""
        self._current_node = node.id
        self._total = total
        label = node.label or node.id
        type_tag = node.type.value

        progress = f"[{index}/{total}]" if total else ""
        msg = f"{progress} {label} ({type_tag})"

        if HAS_RICH:
            if self._status is not None:
                self._status.stop()
            self._status = self._console.status(msg, spinner="dots")
            self._status.start()
        else:
            print(f"  ▶ {msg}", flush=True)

    def on_node_end(self, node_id: str, status: str) -> None:
        """Called when a node finishes."""
        self._nodes_done += 1
        icon = "✓" if status == "success" else "✗"

        if HAS_RICH:
            if self._status is not None:
                self._status.stop()
                self._status = None
            self._console.print(f"  {icon} {node_id} → {status}")
        else:
            print(f"  {icon} {node_id} → {status}", flush=True)

    def on_edge(self, source: str, target: str, label: str) -> None:
        """Called when an edge is traversed."""
        if label:
            detail = f"  → {target} [{label}]"
        else:
            detail = f"  → {target}"

        if HAS_RICH and self._status is not None:
            # briefly show in status before next node starts
            self._status.update(detail)
        elif not HAS_RICH:
            print(detail, flush=True)

    def on_retry(self, node_id: str, attempt: int, max_retries: int, delay: float) -> None:
        """Called when a node is being retried."""
        msg = f"  ↻ {node_id} retry {attempt}/{max_retries} (wait {delay:.0f}s)"
        if HAS_RICH and self._status is not None:
            self._status.update(msg)
        else:
            print(msg, flush=True)

    # ── Agent event handler (for codergen inner loop) ───────────────────

    def on_agent_event(self, event: Event) -> None:
        """Handle events from the agent loop inside a codergen node."""
        if not HAS_RICH or self._status is None:
            # For plain mode, only print the most important events
            if event.type == EventType.TOOL_CALL_START:
                tool = event.data.get("tool_name", "?")
                print(f"    ⚙ {tool}", flush=True)
            return

        # Rich mode — update the spinner status line
        node = self._current_node
        if event.type == EventType.TURN_START:
            turn = event.data.get("turn_number", "?")
            self._status.update(f"{node}: turn {turn} — thinking…")

        elif event.type == EventType.LLM_REQUEST:
            model = event.data.get("model", "")
            self._status.update(f"{node}: calling {model}…")

        elif event.type == EventType.LLM_RESPONSE:
            tokens = event.data.get("tokens", 0)
            self._status.update(f"{node}: received {tokens} tokens")

        elif event.type == EventType.TOOL_CALL_START:
            tool = event.data.get("tool_name", "?")
            self._status.update(f"{node}: ⚙ {tool}")

        elif event.type == EventType.TOOL_CALL_END:
            tool = event.data.get("tool_name", "?")
            ok = "✓" if event.data.get("success") else "✗"
            ms = event.data.get("duration_ms", 0)
            self._status.update(f"{node}: {ok} {tool} ({ms:.0f}ms)")

        elif event.type == EventType.TOOL_ERROR:
            tool = event.data.get("tool_name", "?")
            self._status.update(f"{node}: ✗ {tool} error")

        elif event.type == EventType.LOOP_DETECTED:
            self._status.update(f"{node}: loop detected, steering…")

    # ── Lifecycle ───────────────────────────────────────────────────────

    def stop(self) -> None:
        """Clean up any active spinner."""
        if HAS_RICH and self._status is not None:
            self._status.stop()
            self._status = None
