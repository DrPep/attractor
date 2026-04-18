"""In-memory event hub for web UI.

One `EventHub` per process. Runs are keyed by run_id. Each run has a ring
buffer of recent events plus a set of async subscriber queues. Subscribers
receive the buffered history on connect, then live events as they arrive.
"""

from __future__ import annotations

import asyncio
import time
from collections import deque
from dataclasses import dataclass, field
from typing import Any, AsyncIterator


@dataclass
class RunEvent:
    run_id: str
    seq: int
    ts: float
    kind: str
    data: dict[str, Any]

    def to_dict(self) -> dict[str, Any]:
        return {
            "run_id": self.run_id,
            "seq": self.seq,
            "ts": self.ts,
            "kind": self.kind,
            "data": self.data,
        }


@dataclass
class _RunState:
    buffer: deque[RunEvent] = field(default_factory=lambda: deque(maxlen=2000))
    subscribers: set[asyncio.Queue[RunEvent]] = field(default_factory=set)
    next_seq: int = 0
    finished: bool = False
    # Snapshot of graph + per-node state for late-joining clients.
    graph_dot: str = ""
    node_state: dict[str, dict[str, Any]] = field(default_factory=dict)


class EventHub:
    """Process-wide pub/sub keyed by run_id."""

    def __init__(self) -> None:
        self._runs: dict[str, _RunState] = {}
        self._lock = asyncio.Lock()

    def _run(self, run_id: str) -> _RunState:
        state = self._runs.get(run_id)
        if state is None:
            state = _RunState()
            self._runs[run_id] = state
        return state

    def set_graph(self, run_id: str, dot_source: str) -> None:
        self._run(run_id).graph_dot = dot_source

    def snapshot(self, run_id: str) -> dict[str, Any]:
        state = self._runs.get(run_id)
        if state is None:
            return {"run_id": run_id, "known": False}
        return {
            "run_id": run_id,
            "known": True,
            "finished": state.finished,
            "graph_dot": state.graph_dot,
            "node_state": state.node_state,
            "next_seq": state.next_seq,
        }

    def list_runs(self) -> list[str]:
        return list(self._runs.keys())

    def publish(self, run_id: str, kind: str, data: dict[str, Any]) -> RunEvent:
        state = self._run(run_id)
        event = RunEvent(
            run_id=run_id,
            seq=state.next_seq,
            ts=time.time(),
            kind=kind,
            data=data,
        )
        state.next_seq += 1
        state.buffer.append(event)
        self._apply_to_node_state(state, event)
        # Fan out — best-effort, non-blocking. Slow subscribers just drop.
        for q in list(state.subscribers):
            try:
                q.put_nowait(event)
            except asyncio.QueueFull:
                pass
        return event

    def mark_finished(self, run_id: str) -> None:
        state = self._run(run_id)
        state.finished = True
        self.publish(run_id, "run_end", {})

    async def subscribe(
        self, run_id: str, since_seq: int = 0
    ) -> AsyncIterator[RunEvent]:
        """Yield buffered events since `since_seq`, then live events until run_end."""
        state = self._run(run_id)
        q: asyncio.Queue[RunEvent] = asyncio.Queue(maxsize=1000)
        state.subscribers.add(q)
        try:
            # Replay buffer first (ordered by seq).
            for ev in list(state.buffer):
                if ev.seq >= since_seq:
                    yield ev
            while True:
                ev = await q.get()
                yield ev
                if ev.kind == "run_end":
                    break
        finally:
            state.subscribers.discard(q)

    @staticmethod
    def _apply_to_node_state(state: _RunState, ev: RunEvent) -> None:
        """Fold events into a per-node status snapshot for late-joining clients."""
        d = ev.data
        nid = d.get("node_id")
        if ev.kind == "node_start" and nid:
            entry = state.node_state.setdefault(nid, {})
            entry["status"] = "running"
            entry["started_ts"] = ev.ts
            entry["index"] = d.get("index")
            entry["total"] = d.get("total")
        elif ev.kind == "node_end" and nid:
            entry = state.node_state.setdefault(nid, {})
            entry["status"] = d.get("outcome", "done")
            entry["ended_ts"] = ev.ts
        elif ev.kind == "edge" and d.get("target"):
            tgt = d["target"]
            entry = state.node_state.setdefault(tgt, {})
            entry.setdefault("status", "queued")
        elif ev.kind == "retry" and nid:
            entry = state.node_state.setdefault(nid, {})
            entry["status"] = "retrying"
            entry["attempt"] = d.get("attempt")
