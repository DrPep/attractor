"""Write file tool."""

from __future__ import annotations

from typing import Any, TYPE_CHECKING

from .base import Tool, ToolResult

if TYPE_CHECKING:
    from ..environment import ExecutionEnvironment


class WriteFileTool(Tool):
    @property
    def name(self) -> str:
        return "write_file"

    @property
    def description(self) -> str:
        return "Write content to a file. Creates parent directories if needed."

    @property
    def parameters_schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "file_path": {
                    "type": "string",
                    "description": "Absolute or relative path to write to.",
                },
                "content": {
                    "type": "string",
                    "description": "The content to write to the file.",
                },
            },
            "required": ["file_path", "content"],
        }

    async def execute(
        self, args: dict[str, Any], env: ExecutionEnvironment,
    ) -> ToolResult:
        file_path = args.get("file_path", "")
        content = args.get("content", "")

        try:
            await env.write_file(file_path, content)
            return ToolResult(content=f"Successfully wrote to {file_path}")
        except PermissionError:
            return ToolResult(
                content=f"Permission denied: {file_path}", is_error=True,
            )
        except Exception as e:
            return ToolResult(content=f"Error writing file: {e}", is_error=True)
