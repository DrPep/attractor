"""Tests for feedback loop / self-correction mechanisms."""

from __future__ import annotations

import pytest

from attractor.pipeline.context import PipelineContext
from attractor.pipeline.graph import Edge, Graph, Node, NodeType
from attractor.pipeline.runner import PipelineRunner, RunResult


# ── PipelineContext.keys ─────────────────────────────────────────────────


def test_context_keys_flat():
    ctx = PipelineContext({"a": 1, "b": 2})
    assert sorted(ctx.keys()) == ["a", "b"]


def test_context_keys_nested():
    ctx = PipelineContext()
    ctx.set("code.response", "hello")
    ctx.set("test.response", "fail")
    ctx.set("top", "val")
    keys = ctx.keys()
    assert "code.response" in keys
    assert "test.response" in keys
    assert "top" in keys


def test_context_keys_empty():
    ctx = PipelineContext()
    assert ctx.keys() == []


# ── Node.max_iterations ─────────────────────────────────────────────────


def test_node_max_iterations_default():
    node = Node(id="n", attrs={})
    assert node.max_iterations == 0


def test_node_max_iterations_set():
    node = Node(id="n", attrs={"max_iterations": 5})
    assert node.max_iterations == 5


# ── Backward edges / feedback routing ───────────────────────────────────


@pytest.mark.asyncio
async def test_backward_edge_routing(tmp_run_dir):
    """A node that fails should route back via a backward edge with condition."""
    call_counts: dict[str, int] = {}

    class CountingRunner(PipelineRunner):
        """Track how many times each node is visited."""

    graph = Graph(
        nodes={
            "start": Node(id="start", type=NodeType.START),
            "code": Node(
                id="code", type=NodeType.CODERGEN,
                attrs={"prompt": "write code", "max_iterations": 3},
            ),
            "check": Node(id="check", type=NodeType.CONDITIONAL),
            "done": Node(id="done", type=NodeType.EXIT),
        },
        edges=[
            Edge(source="start", target="code"),
            Edge(source="code", target="check"),
            # On first pass, fail → loop back. On later passes, succeed → done.
            Edge(source="check", target="code", label="retry",
                 condition="context.internal.node_iteration.code!=3"),
            Edge(source="check", target="done",
                 condition="context.internal.node_iteration.code=3"),
        ],
    )

    visited: list[str] = []
    runner = PipelineRunner(
        client=None,  # no LLM — codergen with no client returns fail unless auto_status
        on_node_start=lambda n: visited.append(n),
    )

    # With no client, codergen fails (auto_status=False).
    # But conditional node always succeeds, and routes based on iteration count.
    # We need auto_status on code so it doesn't fail the pipeline
    graph.nodes["code"].attrs["auto_status"] = True

    result = await runner.run(
        _graph_to_dot(graph), run_dir=tmp_run_dir,
    )

    # Code should have been visited 3 times (max_iterations=3)
    code_visits = [v for v in visited if v == "code"]
    assert len(code_visits) == 3
    assert result.success


@pytest.mark.asyncio
async def test_max_iterations_stops_loop(tmp_run_dir):
    """max_iterations on a node should stop re-entry beyond the limit."""
    graph = Graph(
        nodes={
            "start": Node(id="start", type=NodeType.START),
            "work": Node(
                id="work", type=NodeType.CODERGEN,
                attrs={"prompt": "do work", "auto_status": True, "max_iterations": 2},
            ),
            "check": Node(id="check", type=NodeType.CONDITIONAL),
            "done": Node(id="done", type=NodeType.EXIT),
        },
        edges=[
            Edge(source="start", target="work"),
            Edge(source="work", target="check"),
            Edge(source="check", target="work", label="retry"),  # always loops back
            # No edge to done from check — this should eventually hit max_iterations
        ],
    )

    runner = PipelineRunner(client=None)
    result = await runner.run(_graph_to_dot(graph), run_dir=tmp_run_dir)

    assert not result.success
    assert any("max_iterations" in e for e in result.errors)


@pytest.mark.asyncio
async def test_completed_nodes_not_duplicated(tmp_run_dir):
    """When a node is visited multiple times, completed_nodes should not have duplicates."""
    graph = Graph(
        nodes={
            "start": Node(id="start", type=NodeType.START),
            "work": Node(
                id="work", type=NodeType.CODERGEN,
                attrs={"prompt": "do work", "auto_status": True, "max_iterations": 3},
            ),
            "check": Node(id="check", type=NodeType.CONDITIONAL),
            "done": Node(id="done", type=NodeType.EXIT),
        },
        edges=[
            Edge(source="start", target="work"),
            Edge(source="work", target="check"),
            Edge(source="check", target="work",
                 condition="context.internal.node_iteration.work!=3"),
            Edge(source="check", target="done",
                 condition="context.internal.node_iteration.work=3"),
        ],
    )

    runner = PipelineRunner(client=None)
    result = await runner.run(_graph_to_dot(graph), run_dir=tmp_run_dir)

    assert result.success
    # nodes_executed has all visits (including repeats)
    assert result.nodes_executed.count("work") == 3
    # But the final context should show the node was visited 3 times
    assert result.final_context.get("internal", {}).get("node_iteration", {}).get("work") == 3


# ── Helpers ──────────────────────────────────────────────────────────────


def _graph_to_dot(graph: Graph) -> str:
    """Convert a Graph back to DOT source for the parser."""
    lines = [f"digraph {graph.name or 'test'} {{"]
    if graph.goal:
        lines.append(f'    goal="{graph.goal}";')

    for node in graph.nodes.values():
        shape = _type_to_shape(node.type)
        attrs = [f'shape={shape}']
        for k, v in node.attrs.items():
            if isinstance(v, bool):
                attrs.append(f'{k}={"true" if v else "false"}')
            elif isinstance(v, str):
                attrs.append(f'{k}="{v}"')
            else:
                attrs.append(f'{k}={v}')
        lines.append(f'    {node.id} [{", ".join(attrs)}];')

    for edge in graph.edges:
        attrs = []
        if edge.label:
            attrs.append(f'label="{edge.label}"')
        if edge.condition:
            attrs.append(f'condition="{edge.condition}"')
        if edge.weight:
            attrs.append(f'weight={edge.weight}')
        attr_str = f" [{', '.join(attrs)}]" if attrs else ""
        lines.append(f"    {edge.source} -> {edge.target}{attr_str};")

    lines.append("}")
    return "\n".join(lines)


def _type_to_shape(node_type: NodeType) -> str:
    from attractor.pipeline.graph import SHAPE_TO_TYPE
    for shape, nt in SHAPE_TO_TYPE.items():
        if nt == node_type:
            return shape
    return "box"
