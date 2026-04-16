"""DOT file parser using pydot."""

from __future__ import annotations

import re
from pathlib import Path
from typing import Any

from ..exceptions import ParseError
from .graph import Edge, Graph, Node, NodeType, SHAPE_TO_TYPE, parse_attr_value

# Matches valid bare identifiers per spec: [A-Za-z_][A-Za-z0-9_]*
_VALID_NODE_ID = re.compile(r'^[A-Za-z_][A-Za-z0-9_]*$')


def _strip_comments(dot_string: str) -> str:
    """Remove // line comments and /* block */ comments."""
    # Block comments first
    result = re.sub(r'/\*.*?\*/', '', dot_string, flags=re.DOTALL)
    # Line comments
    result = re.sub(r'//[^\n]*', '', result)
    return result


def _pre_validate(dot_string: str) -> None:
    """Enforce spec constraints before handing to pydot.

    Checks:
    - One digraph per file (no multiple graphs)
    - No strict modifier
    - No undirected graphs or edges (--)
    - Bare identifiers only for node IDs
    - Commas required between attributes
    """
    stripped = _strip_comments(dot_string)

    # Reject strict modifier
    if re.search(r'(?i)\bstrict\b', stripped):
        raise ParseError("'strict' modifier is not supported")

    # Reject undirected graph keyword
    # Match 'graph' at top level (not 'digraph', not inside attribute blocks)
    # Look for 'graph' as a standalone keyword before '{'
    header = stripped.split('{', 1)[0] if '{' in stripped else stripped
    header_tokens = header.strip().split()
    if header_tokens and header_tokens[0].lower() == 'graph':
        raise ParseError("Only digraph is supported (undirected graph rejected)")

    # Reject multiple digraphs
    digraph_count = len(re.findall(r'\bdigraph\b', stripped, re.IGNORECASE))
    if digraph_count > 1:
        raise ParseError("Only one digraph per file is allowed")
    if digraph_count == 0:
        raise ParseError("No digraph found in input")

    # Reject undirected edge operator (--)
    # Must avoid matching inside quoted strings. Scan outside quotes only.
    in_quotes = False
    quote_char = None
    i = 0
    while i < len(stripped):
        ch = stripped[i]
        if in_quotes:
            if ch == '\\':
                i += 2
                continue
            if ch == quote_char:
                in_quotes = False
        else:
            if ch in ('"', "'"):
                in_quotes = True
                quote_char = ch
            elif ch == '-' and i + 1 < len(stripped) and stripped[i + 1] == '-':
                # Check it's not -> (already past that since -> has > not -)
                if i + 2 >= len(stripped) or stripped[i + 2] != '>':
                    raise ParseError(
                        "Undirected edges (--) are not supported, use -> for directed edges"
                    )
        i += 1

    # Validate node IDs are bare identifiers and commas separate attributes.
    # Extract attribute blocks [...] and node declarations.
    _validate_node_ids_and_attrs(stripped)


def _validate_node_ids_and_attrs(stripped: str) -> None:
    """Validate node IDs and attribute comma separation."""
    # Find the body between the first { and last }
    brace_start = stripped.find('{')
    brace_end = stripped.rfind('}')
    if brace_start < 0 or brace_end < 0:
        return
    body = stripped[brace_start + 1:brace_end]

    # Validate attribute blocks have commas between key-value pairs
    for attr_match in re.finditer(r'\[([^\]]*)\]', body):
        attr_content = attr_match.group(1).strip()
        if not attr_content:
            continue
        _validate_attr_commas(attr_content, attr_match.start())

    # To find quoted node IDs without matching attribute values,
    # strip out attribute blocks and graph-level key=value assignments,
    # then check any remaining quoted strings.
    body_no_attrs = re.sub(r'\[[^\]]*\]', '', body)
    # Remove graph-level attribute assignments (key = "value")
    body_no_attrs = re.sub(
        r'[A-Za-z_][A-Za-z0-9_]*\s*=\s*"(?:[^"\\]|\\.)*"', '', body_no_attrs,
    )

    # Any remaining quoted strings are node IDs — validate them
    for match in re.finditer(r'"((?:[^"\\]|\\.)*)"', body_no_attrs):
        quoted_id = match.group(1)
        if not _VALID_NODE_ID.match(quoted_id):
            raise ParseError(
                f"Node ID '{quoted_id}' is not a valid bare identifier. "
                f"IDs must match [A-Za-z_][A-Za-z0-9_]*. "
                f"Use the 'label' attribute for human-readable names."
            )


def _validate_attr_commas(attr_content: str, offset: int) -> None:
    """Validate that attributes inside [...] are comma-separated."""
    # Split on commas (respecting quotes) and check that each piece has
    # at most one key=value pair. If we find two key=value pairs in one
    # segment, commas are missing.
    #
    # Strategy: find all key=value pairs, then check that between consecutive
    # pairs there is a comma.
    kv_pattern = re.compile(
        r'([A-Za-z_][A-Za-z0-9_]*)\s*=\s*'
        r'(?:"(?:[^"\\]|\\.)*"|[^,\]\s]+)'
    )
    matches = list(kv_pattern.finditer(attr_content))
    if len(matches) <= 1:
        return

    for i in range(len(matches) - 1):
        between = attr_content[matches[i].end():matches[i + 1].start()]
        # There must be a comma in the gap between consecutive kv pairs
        if ',' not in between:
            raise ParseError(
                f"Attributes must be comma-separated. Missing comma between "
                f"'{matches[i].group().strip()}' and '{matches[i + 1].group().strip()}'"
            )


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

    # Pre-validate against spec constraints before pydot parsing
    _pre_validate(dot_string)

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

    # Validate all node IDs are bare identifiers
    all_node_ids = set(graph.nodes.keys())
    for edge in graph.edges:
        for node_id in (edge.source, edge.target):
            all_node_ids.add(node_id)

    for node_id in all_node_ids:
        if not _VALID_NODE_ID.match(node_id):
            raise ParseError(
                f"Node ID '{node_id}' is not a valid bare identifier. "
                f"IDs must match [A-Za-z_][A-Za-z0-9_]*. "
                f"Use the 'label' attribute for human-readable names."
            )

    # Ensure all edge endpoints have node entries
    for edge in graph.edges:
        for node_id in (edge.source, edge.target):
            if node_id not in graph.nodes:
                graph.nodes[node_id] = Node(
                    id=node_id, label=node_id, type=NodeType.CODERGEN,
                )

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
