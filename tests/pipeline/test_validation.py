"""Tests for graph validation."""

import pytest

from attractor.pipeline.validation import validate, validate_or_raise, Severity
from attractor.pipeline.graph import Edge, Graph, Node, NodeType
from attractor.exceptions import AttractorValidationError


def make_valid_graph():
    return Graph(
        nodes={
            "start": Node(id="start", type=NodeType.START),
            "work": Node(id="work", type=NodeType.CODERGEN, attrs={"prompt": "do work"}),
            "end": Node(id="end", type=NodeType.EXIT),
        },
        edges=[
            Edge(source="start", target="work"),
            Edge(source="work", target="end"),
        ],
    )


def test_valid_graph():
    graph = make_valid_graph()
    diagnostics = validate(graph)
    errors = [d for d in diagnostics if d.severity == Severity.ERROR]
    assert len(errors) == 0


def test_no_start_node():
    graph = Graph(
        nodes={
            "work": Node(id="work", type=NodeType.CODERGEN),
            "end": Node(id="end", type=NodeType.EXIT),
        },
        edges=[Edge(source="work", target="end")],
    )
    diagnostics = validate(graph)
    errors = [d for d in diagnostics if d.severity == Severity.ERROR]
    assert any("start" in d.message.lower() for d in errors)


def test_no_exit_node():
    graph = Graph(
        nodes={
            "start": Node(id="start", type=NodeType.START),
            "work": Node(id="work", type=NodeType.CODERGEN),
        },
        edges=[Edge(source="start", target="work")],
    )
    diagnostics = validate(graph)
    errors = [d for d in diagnostics if d.severity == Severity.ERROR]
    assert any("exit" in d.message.lower() for d in errors)


def test_start_with_incoming_edges():
    graph = Graph(
        nodes={
            "start": Node(id="start", type=NodeType.START),
            "work": Node(id="work", type=NodeType.CODERGEN),
            "end": Node(id="end", type=NodeType.EXIT),
        },
        edges=[
            Edge(source="start", target="work"),
            Edge(source="work", target="start"),  # bad!
            Edge(source="work", target="end"),
        ],
    )
    diagnostics = validate(graph)
    errors = [d for d in diagnostics if d.severity == Severity.ERROR]
    assert any("incoming" in d.message.lower() for d in errors)


def test_unreachable_node_warning():
    graph = Graph(
        nodes={
            "start": Node(id="start", type=NodeType.START),
            "work": Node(id="work", type=NodeType.CODERGEN),
            "orphan": Node(id="orphan", type=NodeType.CODERGEN),
            "end": Node(id="end", type=NodeType.EXIT),
        },
        edges=[
            Edge(source="start", target="work"),
            Edge(source="work", target="end"),
        ],
    )
    diagnostics = validate(graph)
    warnings = [d for d in diagnostics if d.severity == Severity.WARNING]
    assert any("unreachable" in d.message.lower() for d in warnings)


def test_validate_or_raise_on_error():
    graph = Graph(nodes={}, edges=[])
    with pytest.raises(AttractorValidationError):
        validate_or_raise(graph)


def test_validate_or_raise_returns_warnings():
    graph = make_valid_graph()
    # Add orphan for warning
    graph.nodes["orphan"] = Node(id="orphan", type=NodeType.CODERGEN)
    warnings = validate_or_raise(graph)
    assert len(warnings) > 0


def test_missing_prompt_warning():
    graph = Graph(
        nodes={
            "start": Node(id="start", type=NodeType.START),
            "work": Node(id="work", type=NodeType.CODERGEN),  # no prompt
            "end": Node(id="end", type=NodeType.EXIT),
        },
        edges=[
            Edge(source="start", target="work"),
            Edge(source="work", target="end"),
        ],
    )
    diagnostics = validate(graph)
    warnings = [d for d in diagnostics if d.severity == Severity.WARNING]
    assert any("prompt" in d.message.lower() for d in warnings)


def test_unknown_skill_warning():
    graph = Graph(
        nodes={
            "start": Node(id="start", type=NodeType.START),
            "work": Node(
                id="work", type=NodeType.CODERGEN,
                attrs={"prompt": "do work", "skills": "code-review,nonexistent"},
            ),
            "end": Node(id="end", type=NodeType.EXIT),
        },
        edges=[
            Edge(source="start", target="work"),
            Edge(source="work", target="end"),
        ],
    )
    diagnostics = validate(graph, known_skills={"code-review"})
    warnings = [d for d in diagnostics if d.severity == Severity.WARNING]
    assert any("nonexistent" in d.message for d in warnings)
    assert not any("code-review" in d.message for d in warnings)


def test_no_skill_warning_without_known_skills():
    graph = Graph(
        nodes={
            "start": Node(id="start", type=NodeType.START),
            "work": Node(
                id="work", type=NodeType.CODERGEN,
                attrs={"prompt": "do work", "skills": "anything"},
            ),
            "end": Node(id="end", type=NodeType.EXIT),
        },
        edges=[
            Edge(source="start", target="work"),
            Edge(source="work", target="end"),
        ],
    )
    # Without known_skills, no skill warnings should be emitted
    diagnostics = validate(graph)
    assert not any("skill" in d.message.lower() for d in diagnostics)
