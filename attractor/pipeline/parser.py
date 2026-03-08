"""DOT file parser using pydot."""

from __future__ import annotations

from pathlib import Path
from typing import Any

from ..exceptions import ParseError
from .graph import Edge, Graph, Node, NodeType, SHAPE_TO_TYPE, parse_attr_value


def parse_dot(source: str | Path) -> Graph:
    """Parse a DOT file or string into a Graph.

    Args:
        source: Either a file path or a DOT string.
    """
    import pydot

    if isinstance(source, Path) or (
        isinstance(source, str) and not source.strip().startswith("digraph")
        and Path(source).exists()
    ):
        path = Path(source)
        dot_string = path.read_text(encoding="utf-8")
    else:
        dot_string = source

    try:
        graphs = pydot.graph_from_dot_data(dot_string)
    except Exception as e:
        raise ParseError(f"Failed to parse DOT: {e}") from e

    if not graphs:
        raise ParseError("No graph found in DOT input")

    dot_graph = graphs[0]

    if dot_graph.get_type() != "digraph":
        raise ParseError("Only digraph is supported")

    graph = Graph(name=dot_graph.get_name().strip('"'))

    # Extract graph-level attributes
    graph_attrs = _extract_attrs(dot_graph.get_graph_defaults())
    for key, val in dot_graph.obj_dict.get("attributes", {}).items():
        graph_attrs[key] = parse_attr_value(str(val))

    graph.goal = str(graph_attrs.get("goal", ""))
    graph.model_stylesheet = str(graph_attrs.get("model_stylesheet", ""))
    graph.default_max_retry = int(graph_attrs.get("default_max_retry", 50))
    graph.default_fidelity = str(graph_attrs.get("default_fidelity", "compact"))
    graph.retry_target = str(graph_attrs.get("retry_target", ""))
    graph.fallback_retry_target = str(graph_attrs.get("fallback_retry_target", ""))
    graph.attrs = graph_attrs

    # Process subgraph defaults
    subgraph_defaults: dict[str, dict[str, Any]] = {}
    for sg in dot_graph.get_subgraphs():
        sg_name = sg.get_name().strip('"').lstrip("cluster_")
        defaults = _extract_attrs(sg.get_node_defaults())
        subgraph_defaults[sg_name] = defaults
        # Process nodes within subgraph
        for dot_node in sg.get_nodes():
            node = _parse_node(dot_node, defaults)
            if node:
                graph.nodes[node.id] = node
        # Process edges within subgraph
        for dot_edge in sg.get_edges():
            edges = _parse_edge(dot_edge)
            graph.edges.extend(edges)

    # Process top-level nodes
    node_defaults = _extract_attrs(dot_graph.get_node_defaults())
    for dot_node in dot_graph.get_nodes():
        node = _parse_node(dot_node, node_defaults)
        if node and node.id not in graph.nodes:
            graph.nodes[node.id] = node

    # Process top-level edges
    for dot_edge in dot_graph.get_edges():
        edges = _parse_edge(dot_edge)
        graph.edges.extend(edges)

    # Ensure all edge endpoints have node entries
    all_node_ids = set(graph.nodes.keys())
    for edge in graph.edges:
        for node_id in (edge.source, edge.target):
            if node_id not in all_node_ids:
                graph.nodes[node_id] = Node(
                    id=node_id, label=node_id, type=NodeType.CODERGEN,
                )
                all_node_ids.add(node_id)

    return graph


def _extract_attrs(defaults: list) -> dict[str, Any]:
    """Extract attributes from pydot defaults list."""
    attrs: dict[str, Any] = {}
    for default in defaults:
        if hasattr(default, "get_attributes"):
            for key, val in default.get_attributes().items():
                attrs[key] = parse_attr_value(str(val))
        elif isinstance(default, dict):
            for key, val in default.items():
                attrs[key] = parse_attr_value(str(val))
    return attrs


def _parse_node(dot_node: Any, defaults: dict[str, Any] | None = None) -> Node | None:
    """Parse a pydot Node into our Node model."""
    node_id = dot_node.get_name().strip('"')

    # Skip special pydot nodes
    if node_id in ("node", "edge", "graph", ""):
        return None

    attrs = dict(defaults or {})
    for key, val in dot_node.get_attributes().items():
        attrs[key] = parse_attr_value(str(val))

    label = str(attrs.pop("label", node_id))
    shape = str(attrs.pop("shape", "box"))

    # Determine node type
    if "type" in attrs:
        type_str = str(attrs.pop("type"))
        try:
            node_type = NodeType(type_str)
        except ValueError:
            node_type = SHAPE_TO_TYPE.get(shape, NodeType.CODERGEN)
    else:
        node_type = SHAPE_TO_TYPE.get(shape, NodeType.CODERGEN)

    return Node(id=node_id, type=node_type, label=label, attrs=attrs)


def _parse_edge(dot_edge: Any) -> list[Edge]:
    """Parse a pydot Edge into Edge model(s)."""
    source = dot_edge.get_source().strip('"')
    target = dot_edge.get_destination().strip('"')

    attrs: dict[str, Any] = {}
    for key, val in dot_edge.get_attributes().items():
        attrs[key] = parse_attr_value(str(val))

    edge = Edge(
        source=source,
        target=target,
        label=str(attrs.get("label", "")),
        condition=str(attrs.get("condition", "")),
        weight=int(attrs.get("weight", 0)),
        fidelity=str(attrs.get("fidelity", "")),
        thread_id=str(attrs.get("thread_id", "")),
        loop_restart=bool(attrs.get("loop_restart", False)),
    )
    return [edge]
