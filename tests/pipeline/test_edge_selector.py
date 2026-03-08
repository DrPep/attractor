"""Tests for edge selection algorithm."""

from attractor.pipeline.edge_selector import select_edge
from attractor.pipeline.graph import Edge
from attractor.pipeline.context import PipelineContext


def test_single_edge():
    edges = [Edge(source="a", target="b")]
    result = select_edge(edges)
    assert result is not None
    assert result.target == "b"


def test_condition_filtering():
    ctx = PipelineContext()
    edges = [
        Edge(source="a", target="b", condition="outcome = success"),
        Edge(source="a", target="c", condition="outcome = fail"),
    ]
    result = select_edge(edges, outcome="success", context=ctx)
    assert result.target == "b"

    result = select_edge(edges, outcome="fail", context=ctx)
    assert result.target == "c"


def test_preferred_label_match():
    edges = [
        Edge(source="a", target="b", label="Fix"),
        Edge(source="a", target="c", label="Skip"),
    ]
    result = select_edge(edges, preferred_label="Fix")
    assert result.target == "b"


def test_suggested_next_ids():
    edges = [
        Edge(source="a", target="b"),
        Edge(source="a", target="c"),
    ]
    result = select_edge(edges, suggested_next_ids=["c"])
    assert result.target == "c"


def test_weight_priority():
    edges = [
        Edge(source="a", target="b", weight=1),
        Edge(source="a", target="c", weight=10),
    ]
    result = select_edge(edges)
    assert result.target == "c"


def test_lexical_tiebreak():
    edges = [
        Edge(source="a", target="z_node"),
        Edge(source="a", target="a_node"),
    ]
    result = select_edge(edges)
    assert result.target == "a_node"


def test_no_edges():
    assert select_edge([]) is None


def test_no_eligible_edges():
    ctx = PipelineContext()
    edges = [
        Edge(source="a", target="b", condition="outcome = impossible"),
    ]
    result = select_edge(edges, outcome="success", context=ctx)
    assert result is None
