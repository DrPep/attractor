"""Grep content search tool."""

from __future__ import annotations

import re
from typing import Any, TYPE_CHECKING

from .base import Tool, ToolResult

if TYPE_CHECKING:
    from ..environment import ExecutionEnvironment


class GrepTool(Tool):
    @property
    def name(self) -> str:
        return "grep"

    @property
    def description(self) -> str:
        return "Search file contents using regex patterns. Returns matching lines with file paths and line numbers."

    @property
    def parameters_schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "pattern": {
                    "type": "string",
                    "description": "Regex pattern to search for.",
                },
                "path": {
                    "type": "string",
                    "description": "File or directory to search in.",
                },
                "glob": {
                    "type": "string",
                    "description": "Glob filter for files (e.g. '*.py').",
                },
                "context_lines": {
                    "type": "integer",
                    "description": "Number of context lines around matches.",
                },
            },
            "required": ["pattern"],
        }

    async def execute(
        self, args: dict[str, Any], env: ExecutionEnvironment,
    ) -> ToolResult:
        pattern = args.get("pattern", "")
        search_path = args.get("path") or str(env.working_directory)
        file_glob = args.get("glob", "**/*")
        context = args.get("context_lines", 0)

        # Build grep/rg command
        cmd_parts = ["rg", "--line-number", "--no-heading"]
        if context:
            cmd_parts.extend(["-C", str(context)])
        if file_glob and file_glob != "**/*":
            cmd_parts.extend(["--glob", file_glob])
        cmd_parts.extend(["--", pattern, search_path])

        result = await env.run_command(
            " ".join(f'"{p}"' if " " in p else p for p in cmd_parts),
            timeout=30.0,
        )

        if result.exit_code == 0 and result.stdout:
            # Limit output
            lines = result.stdout.splitlines()
            if len(lines) > 200:
                output = "\n".join(lines[:200])
                output += f"\n... ({len(lines) - 200} more lines)"
            else:
                output = result.stdout
            return ToolResult(content=output)
        elif result.exit_code == 1:
            return ToolResult(content="No matches found.")
        elif result.exit_code == 2 or "not found" in result.stderr.lower():
            # rg not available, fall back to grep
            cmd_parts = ["grep", "-rn"]
            if context:
                cmd_parts.extend([f"-C{context}"])
            if file_glob and file_glob != "**/*":
                cmd_parts.extend(["--include", file_glob])
            cmd_parts.extend(["--", pattern, search_path])
            result = await env.run_command(
                " ".join(f'"{p}"' if " " in p else p for p in cmd_parts),
                timeout=30.0,
            )
            if result.stdout:
                return ToolResult(content=result.stdout[:50000])
            return ToolResult(content="No matches found.")
        else:
            return ToolResult(
                content=f"Search error: {result.stderr}", is_error=True,
            )
