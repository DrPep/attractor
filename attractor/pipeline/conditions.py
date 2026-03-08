"""Condition expression language evaluator.

Supports: = (equals), != (not equals), && (AND conjunction).
Variables: outcome, preferred_label, context.* (missing = empty string).
Empty condition always returns True.
"""

from __future__ import annotations

import re
from typing import Any

from .context import PipelineContext
from ..exceptions import AttractorValidationError


def evaluate(
    expression: str,
    context: PipelineContext,
    outcome: str = "",
    preferred_label: str = "",
) -> bool:
    """Evaluate a condition expression against context and outcome."""
    expr = expression.strip()
    if not expr:
        return True

    # Split on && (AND conjunction)
    clauses = [c.strip() for c in expr.split("&&")]
    return all(_eval_clause(c, context, outcome, preferred_label) for c in clauses)


def _eval_clause(
    clause: str,
    context: PipelineContext,
    outcome: str,
    preferred_label: str,
) -> bool:
    """Evaluate a single clause (comparison)."""
    # Try != first (before =)
    if "!=" in clause:
        parts = clause.split("!=", 1)
        if len(parts) == 2:
            left = _resolve_value(parts[0].strip(), context, outcome, preferred_label)
            right = _resolve_value(parts[1].strip(), context, outcome, preferred_label)
            return str(left) != str(right)

    if "=" in clause:
        parts = clause.split("=", 1)
        if len(parts) == 2:
            left = _resolve_value(parts[0].strip(), context, outcome, preferred_label)
            right = _resolve_value(parts[1].strip(), context, outcome, preferred_label)
            return str(left) == str(right)

    raise AttractorValidationError(f"Invalid condition clause: {clause}")


def _resolve_value(
    token: str,
    context: PipelineContext,
    outcome: str,
    preferred_label: str,
) -> str:
    """Resolve a value token to its string value."""
    token = token.strip()

    # Strip quotes if present
    if (token.startswith('"') and token.endswith('"')) or \
       (token.startswith("'") and token.endswith("'")):
        return token[1:-1]

    # Built-in variables
    if token == "outcome":
        return outcome
    if token == "preferred_label":
        return preferred_label

    # Context lookup
    if token.startswith("context."):
        key = token[8:]  # Remove "context."
        val = context.get(key, "")
        return str(val) if val is not None else ""

    # Literal values
    return token


def validate_condition(expression: str) -> list[str]:
    """Validate condition syntax. Returns list of error messages."""
    errors: list[str] = []
    expr = expression.strip()
    if not expr:
        return errors

    clauses = [c.strip() for c in expr.split("&&")]
    for clause in clauses:
        if "!=" in clause:
            parts = clause.split("!=", 1)
        elif "=" in clause:
            parts = clause.split("=", 1)
        else:
            errors.append(f"Clause missing operator (= or !=): '{clause}'")
            continue

        if len(parts) != 2 or not parts[0].strip() or not parts[1].strip():
            errors.append(f"Invalid clause: '{clause}'")

    return errors
