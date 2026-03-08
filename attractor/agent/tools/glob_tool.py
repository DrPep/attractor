"""Glob file matching tool."""

from __future__ import annotations

from typing import Any, TYPE_CHECKING

from .base import Tool, ToolResult

if TYPE_CHECKING:
    from ..environment import ExecutionEnvironment


class GlobTool(Tool):
    @property
    def name(self) -> str:
        return "glob"

    @property
    def description(self) -> str:
        return "Find files matching a glob pattern (e.g. '**/*.py', 'src/**/*.ts')."

    @property
    def parameters_schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "pattern": {
                    "type": "string",
                    "description": "The glob pattern to match files against.",
                },
                "path": {
                    "type": "string",
                    "description": "Base directory to search in.",
                },
            },
            "required": ["pattern"],
        }

    async def execute(
        self, args: dict[str, Any], env: ExecutionEnvironment,
    ) -> ToolResult:
        pattern = args.get("pattern", "")
        base_path = args.get("path")

        try:
            files = await env.list_files(pattern, base_path)
            if not files:
                return ToolResult(content="No files matched the pattern.")
            return ToolResult(content="\n".join(files))
        except Exception as e:
            return ToolResult(content=f"Error: {e}", is_error=True)
