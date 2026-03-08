"""CSS-like model stylesheet parser and applier."""

from __future__ import annotations

import re
from typing import Any

from pydantic import BaseModel, Field

from .graph import Graph, Node


class StyleRule(BaseModel):
    """A single stylesheet rule."""
    selector_type: str = ""  # "*", "class", "id"
    selector_value: str = ""  # class name or node ID
    properties: dict[str, str] = Field(default_factory=dict)
    specificity: int = 0  # 0=universal, 1=class, 2=id


def parse_stylesheet(css_text: str) -> list[StyleRule]:
    """Parse CSS-like model stylesheet text into rules."""
    rules: list[StyleRule] = []
    if not css_text.strip():
        return rules

    # Match: selector { property: value; ... }
    pattern = re.compile(
        r'([^{]+)\{([^}]*)\}',
        re.DOTALL,
    )

    for match in pattern.finditer(css_text):
        selector_raw = match.group(1).strip()
        body = match.group(2).strip()

        # Parse selector
        if selector_raw == "*":
            sel_type, sel_value, specificity = "*", "*", 0
        elif selector_raw.startswith("#"):
            sel_type, sel_value, specificity = "id", selector_raw[1:], 2
        elif selector_raw.startswith("."):
            sel_type, sel_value, specificity = "class", selector_raw[1:], 1
        else:
            sel_type, sel_value, specificity = "class", selector_raw, 1

        # Parse properties
        properties: dict[str, str] = {}
        for prop_match in re.finditer(r'([\w_-]+)\s*:\s*"?([^";]+)"?\s*;', body):
            key = prop_match.group(1).strip()
            value = prop_match.group(2).strip().strip('"')
            properties[key] = value

        if properties:
            rules.append(StyleRule(
                selector_type=sel_type,
                selector_value=sel_value,
                properties=properties,
                specificity=specificity,
            ))

    return rules


def apply_stylesheet(rules: list[StyleRule], graph: Graph) -> Graph:
    """Apply stylesheet rules to graph nodes.

    Specificity order: * < .class < #id.
    Explicit node attributes override stylesheet values.
    """
    # Sort rules by specificity (lowest first)
    sorted_rules = sorted(rules, key=lambda r: r.specificity)

    for node in graph.nodes.values():
        resolved: dict[str, str] = {}

        for rule in sorted_rules:
            if _matches(rule, node):
                resolved.update(rule.properties)

        # Apply resolved properties, but don't override explicit node attrs
        for key, value in resolved.items():
            if key not in node.attrs:
                node.attrs[key] = value

    return graph


def _matches(rule: StyleRule, node: Node) -> bool:
    """Check if a rule's selector matches a node."""
    if rule.selector_type == "*":
        return True
    if rule.selector_type == "id":
        return node.id == rule.selector_value
    if rule.selector_type == "class":
        node_classes = node.class_name.split()
        return rule.selector_value in node_classes or node.type.value == rule.selector_value
    return False
