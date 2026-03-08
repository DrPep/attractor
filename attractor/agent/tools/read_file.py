"""Read file tool."""

from __future__ import annotations

from typing import Any, TYPE_CHECKING

from .base import Tool, ToolResult

if TYPE_CHECKING:
    from ..environment import ExecutionEnvironment


class ReadFileTool(Tool):
    @property
    def name(self) -> str:
        return "read_file"

    @property
    def description(self) -> str:
        return "Read the contents of a file. Returns file content with line numbers."

    @property
    def parameters_schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "file_path": {
                    "type": "string",
                    "description": "Absolute or relative path to the file to read.",
                },
                "offset": {
                    "type": "integer",
                    "description": "Line number to start reading from (1-based).",
                },
                "limit": {
                    "type": "integer",
                    "description": "Maximum number of lines to read.",
                },
            },
            "required": ["file_path"],
        }

    async def execute(
        self, args: dict[str, Any], env: ExecutionEnvironment,
    ) -> ToolResult:
        file_path = args.get("file_path", "")
        offset = args.get("offset", 1)
        limit = args.get("limit", 2000)

        try:
            content = await env.read_file(file_path)
        except FileNotFoundError:
            return ToolResult(
                content=f"File not found: {file_path}", is_error=True,
            )
        except PermissionError:
            return ToolResult(
                content=f"Permission denied: {file_path}", is_error=True,
            )
        except Exception as e:
            return ToolResult(content=f"Error reading file: {e}", is_error=True)

        lines = content.splitlines()
        start = max(0, (offset or 1) - 1)
        end = start + (limit or 2000)
        selected = lines[start:end]

        numbered = []
        for i, line in enumerate(selected, start=start + 1):
            numbered.append(f"{i:>6}\t{line}")

        return ToolResult(content="\n".join(numbered))
