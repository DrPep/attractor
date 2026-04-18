"""Bridges PipelineRunner callbacks into an EventHub.

The runner already accepts on_node_start / on_node_end / on_edge / on_retry /
on_agent_event. We adapt those into structured events published to the hub.
"""

from __future__ import annotations

from typing import Any

from ..agent.events import Event as AgentEvent
from ..pipeline.graph import Node
from .events import EventHub


def attach_to_runner(hub: EventHub, run_id: str) -> dict[str, Any]:
    """Return a dict of callbacks suitable for `PipelineRunner(**callbacks)`."""

    def on_node_start(node: Node, index: int, total: int) -> None:
        hub.publish(run_id, "node_start", {
            "node_id": node.id,
            "label": node.label or node.id,
            "type": node.type.value,
            "index": index,
            "total": total,
        })

    def on_node_end(node_id: str, status: str) -> None:
        hub.publish(run_id, "node_end", {
            "node_id": node_id,
            "outcome": status,
        })

    def on_edge(source: str, target: str, label: str) -> None:
        hub.publish(run_id, "edge", {
            "source": source,
            "target": target,
            "label": label,
        })

    def on_retry(node_id: str, attempt: int, max_retries: int, delay: float) -> None:
        hub.publish(run_id, "retry", {
            "node_id": node_id,
            "attempt": attempt,
            "max_retries": max_retries,
            "delay": delay,
        })

    def on_agent_event(event: AgentEvent) -> None:
        hub.publish(run_id, "agent_event", {
            "type": event.type.value,
            "payload": event.data,
        })

    return {
        "on_node_start": on_node_start,
        "on_node_end": on_node_end,
        "on_edge": on_edge,
        "on_retry": on_retry,
        "on_agent_event": on_agent_event,
    }
