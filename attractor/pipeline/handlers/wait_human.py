"""Wait-for-human handler."""

from __future__ import annotations

from pathlib import Path

from ..context import PipelineContext
from ..graph import Graph, Node
from ..interviewer import Answer, Interviewer, Question, QuestionType
from .base import Handler, Outcome


class WaitHumanHandler(Handler):
    """Presents edge labels as choices via interviewer."""

    def __init__(self, interviewer: Interviewer):
        self._interviewer = interviewer

    async def execute(
        self, node: Node, context: PipelineContext,
        graph: Graph, run_dir: Path,
    ) -> Outcome:
        # Get outgoing edges as choices
        outgoing = graph.get_outgoing_edges(node.id)
        choices = [e.label or e.target for e in outgoing if e.label or e.target]

        if not choices:
            return Outcome(status="success", notes="No choices available")

        prompt = node.attrs.get("prompt", node.label) or f"Choose next step for '{node.id}'"

        question = Question(
            text=str(prompt),
            type=QuestionType.SINGLE_SELECT,
            choices=choices,
            timeout=node.timeout,
        )

        answer = await self._interviewer.ask(question)
        selected = answer.value if isinstance(answer.value, str) else str(answer.value)

        return Outcome(
            status="success",
            preferred_label=selected,
            notes=f"Human selected: {selected}",
        )
