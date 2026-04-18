"""Graph validation and linting."""

from __future__ import annotations

from enum import Enum

from pydantic import BaseModel

from ..exceptions import AttractorValidationError
from .conditions import validate_condition
from .graph import Graph, NodeType


class Severity(str, Enum):
    ERROR = "error"
    WARNING = "warning"
    INFO = "info"


class Diagnostic(BaseModel):
    severity: Severity
    message: str
    node_id: str = ""
    suggested_fix: str = ""


def validate(
    graph: Graph, known_skills: set[str] | None = None,
) -> list[Diagnostic]:
    """Validate a pipeline graph. Returns list of diagnostics."""
    diagnostics: list[Diagnostic] = []

    # Check for start node
    start_nodes = [n for n in graph.nodes.values() if n.type == NodeType.START]
    if len(start_nodes) == 0:
        diagnostics.append(Diagnostic(
            severity=Severity.ERROR,
            message="No start node found (use shape=Mdiamond)",
            suggested_fix="Add a node with shape=Mdiamond",
        ))
    elif len(start_nodes) > 1:
        diagnostics.append(Diagnostic(
            severity=Severity.ERROR,
            message=f"Multiple start nodes found: {[n.id for n in start_nodes]}",
            suggested_fix="Keep only one start node",
        ))

    # Check for exit node
    exit_nodes = [n for n in graph.nodes.values() if n.type == NodeType.EXIT]
    if len(exit_nodes) == 0:
        diagnostics.append(Diagnostic(
            severity=Severity.ERROR,
            message="No exit node found (use shape=Msquare)",
            suggested_fix="Add a node with shape=Msquare",
        ))

    # Check start node has no incoming edges
    for start in start_nodes:
        incoming = graph.get_incoming_edges(start.id)
        if incoming:
            diagnostics.append(Diagnostic(
                severity=Severity.ERROR,
                message=f"Start node '{start.id}' has incoming edges",
                node_id=start.id,
                suggested_fix="Remove edges pointing to the start node",
            ))

    # Check exit nodes have no outgoing edges
    for exit_node in exit_nodes:
        outgoing = graph.get_outgoing_edges(exit_node.id)
        if outgoing:
            diagnostics.append(Diagnostic(
                severity=Severity.ERROR,
                message=f"Exit node '{exit_node.id}' has outgoing edges",
                node_id=exit_node.id,
                suggested_fix="Remove edges from the exit node",
            ))

    # Check for unreachable nodes
    if start_nodes:
        reachable = _find_reachable(graph, start_nodes[0].id)
        for node_id in graph.nodes:
            if node_id not in reachable:
                diagnostics.append(Diagnostic(
                    severity=Severity.WARNING,
                    message=f"Node '{node_id}' is unreachable from start",
                    node_id=node_id,
                    suggested_fix="Add an edge path from start to this node, or remove it",
                ))

    # Check edge targets exist
    all_ids = set(graph.nodes.keys())
    for edge in graph.edges:
        if edge.source not in all_ids:
            diagnostics.append(Diagnostic(
                severity=Severity.ERROR,
                message=f"Edge source '{edge.source}' does not exist",
            ))
        if edge.target not in all_ids:
            diagnostics.append(Diagnostic(
                severity=Severity.ERROR,
                message=f"Edge target '{edge.target}' does not exist",
            ))

    # Validate condition syntax
    for edge in graph.edges:
        if edge.condition:
            errors = validate_condition(edge.condition)
            for err in errors:
                diagnostics.append(Diagnostic(
                    severity=Severity.ERROR,
                    message=f"Condition syntax error on edge {edge.source}->{edge.target}: {err}",
                ))

    # Warnings for codergen nodes without prompt
    for node in graph.nodes.values():
        if node.type == NodeType.CODERGEN and not node.prompt:
            diagnostics.append(Diagnostic(
                severity=Severity.WARNING,
                message=f"Codergen node '{node.id}' has no prompt",
                node_id=node.id,
                suggested_fix="Add a prompt attribute",
            ))

    # Warn about unknown skill references
    if known_skills is not None:
        for node in graph.nodes.values():
            for skill_name in node.skills:
                if skill_name not in known_skills:
                    diagnostics.append(Diagnostic(
                        severity=Severity.WARNING,
                        message=f"Node '{node.id}' references unknown skill '{skill_name}'",
                        node_id=node.id,
                        suggested_fix=f"Register skill '{skill_name}' or check for typos",
                    ))

    # Warnings for goal_gate nodes without retry_target — mirrors the runtime
    # fallback chain in goal_gate.py: node → graph.retry_target → graph.fallback_retry_target.
    graph_fallback = graph.retry_target or graph.fallback_retry_target
    for node in graph.nodes.values():
        if node.goal_gate and not node.retry_target and not graph_fallback:
            diagnostics.append(Diagnostic(
                severity=Severity.WARNING,
                message=f"Goal gate node '{node.id}' has no retry_target",
                node_id=node.id,
                suggested_fix="Add retry_target on the node, or graph-level retry_target/fallback_retry_target",
            ))

    return diagnostics


def validate_or_raise(
    graph: Graph, known_skills: set[str] | None = None,
) -> list[Diagnostic]:
    """Validate and raise on ERROR diagnostics. Returns warnings/info."""
    diagnostics = validate(graph, known_skills=known_skills)
    errors = [d for d in diagnostics if d.severity == Severity.ERROR]
    if errors:
        msg = "; ".join(d.message for d in errors)
        raise AttractorValidationError(f"Graph validation failed: {msg}")
    return [d for d in diagnostics if d.severity != Severity.ERROR]


def _find_reachable(graph: Graph, start_id: str) -> set[str]:
    """BFS to find all reachable nodes from start."""
    visited: set[str] = set()
    queue = [start_id]
    while queue:
        current = queue.pop(0)
        if current in visited:
            continue
        visited.add(current)
        for edge in graph.get_outgoing_edges(current):
            if edge.target not in visited:
                queue.append(edge.target)
    return visited
