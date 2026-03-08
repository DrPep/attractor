"""Edit file tool - exact string replacement."""

from __future__ import annotations

from typing import Any, TYPE_CHECKING

from .base import Tool, ToolResult

if TYPE_CHECKING:
    from ..environment import ExecutionEnvironment


class EditFileTool(Tool):
    @property
    def name(self) -> str:
        return "edit_file"

    @property
    def description(self) -> str:
        return (
            "Perform exact string replacement in a file. "
            "The old_string must be unique in the file unless replace_all is true."
        )

    @property
    def parameters_schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "file_path": {
                    "type": "string",
                    "description": "Path to the file to edit.",
                },
                "old_string": {
                    "type": "string",
                    "description": "The exact string to find and replace.",
                },
                "new_string": {
                    "type": "string",
                    "description": "The replacement string.",
                },
                "replace_all": {
                    "type": "boolean",
                    "description": "Replace all occurrences (default: false).",
                },
            },
            "required": ["file_path", "old_string", "new_string"],
        }

    async def execute(
        self, args: dict[str, Any], env: ExecutionEnvironment,
    ) -> ToolResult:
        file_path = args.get("file_path", "")
        old_string = args.get("old_string", "")
        new_string = args.get("new_string", "")
        replace_all = args.get("replace_all", False)

        try:
            content = await env.read_file(file_path)
        except FileNotFoundError:
            return ToolResult(
                content=f"File not found: {file_path}", is_error=True,
            )
        except Exception as e:
            return ToolResult(content=f"Error reading file: {e}", is_error=True)

        if old_string not in content:
            return ToolResult(
                content=f"old_string not found in {file_path}", is_error=True,
            )

        if not replace_all:
            count = content.count(old_string)
            if count > 1:
                return ToolResult(
                    content=f"old_string appears {count} times in {file_path}. "
                    "Use replace_all=true or provide more context to make it unique.",
                    is_error=True,
                )
            new_content = content.replace(old_string, new_string, 1)
        else:
            new_content = content.replace(old_string, new_string)

        try:
            await env.write_file(file_path, new_content)
            replacements = content.count(old_string)
            return ToolResult(
                content=f"Replaced {replacements} occurrence(s) in {file_path}",
            )
        except Exception as e:
            return ToolResult(content=f"Error writing file: {e}", is_error=True)
