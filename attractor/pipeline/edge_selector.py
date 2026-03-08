"""5-step edge selection algorithm."""

from __future__ import annotations

from .conditions import evaluate
from .context import PipelineContext
from .graph import Edge


def select_edge(
    edges: list[Edge],
    outcome: str = "",
    preferred_label: str = "",
    suggested_next_ids: list[str] | None = None,
    context: PipelineContext | None = None,
) -> Edge | None:
    """Select the next edge using the 5-step priority algorithm.

    Steps:
    1. Filter edges with conditions, keeping those that evaluate to True
    2. Match preferred_label against edge labels
    3. Match suggested_next_ids against edge targets
    4. Pick highest weight
    5. Lexical tiebreak by target node ID
    """
    if not edges:
        return None

    ctx = context or PipelineContext()
    suggested = suggested_next_ids or []

    # Step 1: Evaluate conditions — keep edges where condition is True
    eligible: list[Edge] = []
    for edge in edges:
        if edge.condition:
            if evaluate(edge.condition, ctx, outcome, preferred_label):
                eligible.append(edge)
        else:
            eligible.append(edge)

    if not eligible:
        return None

    # Step 2: Match preferred_label
    if preferred_label:
        label_matches = [e for e in eligible if e.label == preferred_label]
        if label_matches:
            eligible = label_matches

    # Step 3: Match suggested_next_ids
    if suggested and len(eligible) > 1:
        suggested_matches = [e for e in eligible if e.target in suggested]
        if suggested_matches:
            eligible = suggested_matches

    # Step 4: Highest weight
    if len(eligible) > 1:
        max_weight = max(e.weight for e in eligible)
        eligible = [e for e in eligible if e.weight == max_weight]

    # Step 5: Lexical tiebreak
    if len(eligible) > 1:
        eligible.sort(key=lambda e: e.target)

    return eligible[0]
