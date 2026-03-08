"""Tool handler - executes shell commands or API calls."""

from __future__ import annotations

import asyncio
import logging
from pathlib import Path

from ..context import PipelineContext
from ..graph import Graph, Node
from ..status import write_response
from .base import Handler, Outcome

logger = logging.getLogger(__name__)


class ToolHandler(Handler):
    """Executes a shell command from the node's prompt/command attribute."""

    async def execute(
        self, node: Node, context: PipelineContext,
        graph: Graph, run_dir: Path,
    ) -> Outcome:
        command = node.attrs.get("command", "") or node.prompt
        if not command:
            return Outcome(status="fail", notes="No command specified")

        # Expand $goal and context variables
        goal = context.get("graph.goal", "") or ""
        command = str(command).replace("$goal", str(goal))

        timeout = node.timeout or 120.0

        try:
            proc = await asyncio.create_subprocess_shell(
                command,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                cwd=str(run_dir),
            )
            stdout, stderr = await asyncio.wait_for(
                proc.communicate(), timeout=timeout,
            )

            output = stdout.decode(errors="replace")
            error_output = stderr.decode(errors="replace")

            write_response(node.id, output + error_output, run_dir)

            if proc.returncode == 0:
                return Outcome(
                    status="success",
                    notes=f"Command completed (exit 0)",
                    context_updates={f"{node.id}.output": output.strip()},
                )
            else:
                return Outcome(
                    status="fail",
                    notes=f"Command failed (exit {proc.returncode}): {error_output[:200]}",
                )

        except asyncio.TimeoutError:
            return Outcome(status="fail", notes=f"Command timed out after {timeout}s")
        except Exception as e:
            logger.exception("Tool handler error for node %s", node.id)
            return Outcome(status="fail", notes=str(e))
