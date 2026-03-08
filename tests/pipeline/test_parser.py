"""Tests for DOT parser."""

import pytest

from attractor.pipeline.parser import parse_dot
from attractor.pipeline.graph import NodeType
from attractor.exceptions import ParseError


def test_parse_simple_pipeline():
    dot = '''
    digraph pipeline {
        start [shape=Mdiamond];
        work [shape=box, prompt="Do work"];
        finish [shape=Msquare];
        start -> work -> finish;
    }
    '''
    graph = parse_dot(dot)
    assert "start" in graph.nodes
    assert "work" in graph.nodes
    assert "finish" in graph.nodes
    assert graph.nodes["start"].type == NodeType.START
    assert graph.nodes["work"].type == NodeType.CODERGEN
    assert graph.nodes["finish"].type == NodeType.EXIT
    assert len(graph.edges) == 2


def test_parse_with_goal():
    dot = '''
    digraph test {
        goal = "Build a website";
        s [shape=Mdiamond];
        e [shape=Msquare];
        s -> e;
    }
    '''
    graph = parse_dot(dot)
    assert graph.goal == "Build a website"


def test_parse_edge_attributes():
    dot = '''
    digraph test {
        s [shape=Mdiamond];
        a [shape=box];
        b [shape=box];
        e [shape=Msquare];
        s -> a [label="start"];
        a -> b [condition="outcome = success", weight=10];
        b -> e;
    }
    '''
    graph = parse_dot(dot)
    cond_edges = [e for e in graph.edges if e.condition]
    assert len(cond_edges) == 1
    assert cond_edges[0].condition == "outcome = success"
    assert cond_edges[0].weight == 10


def test_parse_node_attributes():
    dot = '''
    digraph test {
        s [shape=Mdiamond];
        work [shape=box, max_retries=3, goal_gate=true, prompt="Do stuff"];
        e [shape=Msquare];
        s -> work -> e;
    }
    '''
    graph = parse_dot(dot)
    work = graph.nodes["work"]
    assert work.max_retries == 3
    assert work.goal_gate is True
    assert work.prompt == "Do stuff"


def test_parse_hexagon_wait_human():
    dot = '''
    digraph test {
        s [shape=Mdiamond];
        review [shape=hexagon, label="Review"];
        e [shape=Msquare];
        s -> review -> e;
    }
    '''
    graph = parse_dot(dot)
    assert graph.nodes["review"].type == NodeType.WAIT_HUMAN


def test_parse_diamond_conditional():
    dot = '''
    digraph test {
        s [shape=Mdiamond];
        check [shape=diamond, label="Tests pass?"];
        e [shape=Msquare];
        s -> check -> e;
    }
    '''
    graph = parse_dot(dot)
    assert graph.nodes["check"].type == NodeType.CONDITIONAL


def test_invalid_dot_raises():
    with pytest.raises(ParseError):
        parse_dot("not a valid dot file at all {{{")


def test_non_digraph_raises():
    with pytest.raises(ParseError):
        parse_dot("graph G { a -- b; }")
