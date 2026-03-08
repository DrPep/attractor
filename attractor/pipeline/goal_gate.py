"""Goal gate enforcement."""

from __future__ import annotations

from pydantic import BaseModel, Field

from .graph import Graph, NodeType


class GoalGateResult(BaseModel):
    satisfied: bool = True
    unsatisfied_gates: list[str] = Field(default_factory=list)
    retry_target: str = ""


def check_goal_gates(
    graph: Graph,
    completed_outcomes: dict[str, str],
) -> GoalGateResult:
    """Check if all goal gate nodes achieved SUCCESS or PARTIAL_SUCCESS.

    Args:
        graph: The pipeline graph.
        completed_outcomes: Map of node_id -> outcome status string.
    """
    unsatisfied: list[str] = []
    acceptable = {"success", "partial_success"}

    for node in graph.nodes.values():
        if not node.goal_gate:
            continue
        outcome = completed_outcomes.get(node.id, "")
        if outcome not in acceptable:
            unsatisfied.append(node.id)

    if not unsatisfied:
        return GoalGateResult(satisfied=True)

    # Find retry target from the chain
    retry_target = ""
    for gate_id in unsatisfied:
        node = graph.get_node(gate_id)
        if node and node.retry_target:
            retry_target = node.retry_target
            break

    if not retry_target and graph.retry_target:
        retry_target = graph.retry_target
    if not retry_target and graph.fallback_retry_target:
        retry_target = graph.fallback_retry_target

    return GoalGateResult(
        satisfied=False,
        unsatisfied_gates=unsatisfied,
        retry_target=retry_target,
    )
