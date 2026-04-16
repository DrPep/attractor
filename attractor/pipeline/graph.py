"""Graph model: Node, Edge, and Graph with attribute handling."""

from __future__ import annotations

import re
from enum import Enum
from typing import Any

from pydantic import BaseModel, Field


class NodeType(str, Enum):
    START = "start"
    EXIT = "exit"
    CODERGEN = "codergen"
    WAIT_HUMAN = "wait.human"
    CONDITIONAL = "conditional"
    PARALLEL = "parallel"
    FAN_IN = "fan_in"
    TOOL = "tool"
    MANAGER_LOOP = "manager_loop"


SHAPE_TO_TYPE: dict[str, NodeType] = {
    "Mdiamond": NodeType.START,
    "Msquare": NodeType.EXIT,
    "box": NodeType.CODERGEN,
    "hexagon": NodeType.WAIT_HUMAN,
    "diamond": NodeType.CONDITIONAL,
    "component": NodeType.PARALLEL,
    "tripleoctagon": NodeType.FAN_IN,
    "parallelogram": NodeType.TOOL,
    "house": NodeType.MANAGER_LOOP,
}


def parse_duration(value: str) -> float:
    """Parse duration string (e.g., '900s', '15m', '2h', '1d') to seconds."""
    match = re.match(r'^(\d+(?:\.\d+)?)\s*(ms|s|m|h|d)$', value.strip())
    if not match:
        raise ValueError(f"Invalid duration: {value}")
    num = float(match.group(1))
    unit = match.group(2)
    multipliers = {"ms": 0.001, "s": 1, "m": 60, "h": 3600, "d": 86400}
    return num * multipliers[unit]


def parse_attr_value(value: str) -> Any:
    """Parse attribute value to appropriate Python type."""
    v = value.strip().strip('"')
    if v.lower() == "true":
        return True
    if v.lower() == "false":
        return False
    try:
        return int(v)
    except ValueError:
        pass
    try:
        return float(v)
    except ValueError:
        pass
    # Try duration
    try:
        return parse_duration(v)
    except ValueError:
        pass
    return v


class Node(BaseModel):
    id: str
    type: NodeType = NodeType.CODERGEN
    label: str = ""
    attrs: dict[str, Any] = Field(default_factory=dict)

    @property
    def prompt(self) -> str:
        return str(self.attrs.get("prompt", ""))

    @property
    def max_retries(self) -> int:
        return int(self.attrs.get("max_retries", 0))

    @property
    def goal_gate(self) -> bool:
        return bool(self.attrs.get("goal_gate", False))

    @property
    def fidelity(self) -> str:
        return str(self.attrs.get("fidelity", ""))

    @property
    def timeout(self) -> float | None:
        val = self.attrs.get("timeout")
        if val is None:
            return None
        if isinstance(val, (int, float)):
            return float(val)
        return parse_duration(str(val))

    @property
    def auto_status(self) -> bool:
        return bool(self.attrs.get("auto_status", False))

    @property
    def allow_partial(self) -> bool:
        return bool(self.attrs.get("allow_partial", False))

    @property
    def retry_target(self) -> str:
        return str(self.attrs.get("retry_target", ""))

    @property
    def llm_model(self) -> str:
        return str(self.attrs.get("llm_model", ""))

    @property
    def llm_provider(self) -> str:
        return str(self.attrs.get("llm_provider", ""))

    @property
    def reasoning_effort(self) -> str:
        return str(self.attrs.get("reasoning_effort", "high"))

    @property
    def class_name(self) -> str:
        return str(self.attrs.get("class", ""))

    @property
    def max_iterations(self) -> int:
        return int(self.attrs.get("max_iterations", 0))

    @property
    def skills(self) -> list[str]:
        raw = str(self.attrs.get("skills", ""))
        return [s.strip() for s in raw.split(",") if s.strip()]

    @property
    def thread_id(self) -> str:
        return str(self.attrs.get("thread_id", ""))


class Edge(BaseModel):
    source: str
    target: str
    label: str = ""
    condition: str = ""
    weight: int = 0
    fidelity: str = ""
    thread_id: str = ""
    loop_restart: bool = False


class Graph(BaseModel):
    name: str = ""
    nodes: dict[str, Node] = Field(default_factory=dict)
    edges: list[Edge] = Field(default_factory=list)
    goal: str = ""
    model_stylesheet: str = ""
    default_max_retry: int = 50
    default_fidelity: str = "compact"
    retry_target: str = ""
    fallback_retry_target: str = ""
    attrs: dict[str, Any] = Field(default_factory=dict)

    def get_node(self, node_id: str) -> Node | None:
        return self.nodes.get(node_id)

    def get_outgoing_edges(self, node_id: str) -> list[Edge]:
        return [e for e in self.edges if e.source == node_id]

    def get_incoming_edges(self, node_id: str) -> list[Edge]:
        return [e for e in self.edges if e.target == node_id]

    @property
    def start_node(self) -> Node | None:
        for node in self.nodes.values():
            if node.type == NodeType.START:
                return node
        return None

    @property
    def exit_nodes(self) -> list[Node]:
        return [n for n in self.nodes.values() if n.type == NodeType.EXIT]
