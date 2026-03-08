"""Built-in tools for the coding agent."""

from .base import Tool, ToolRegistry, ToolResult
from .read_file import ReadFileTool
from .write_file import WriteFileTool
from .edit_file import EditFileTool
from .shell import ShellTool
from .glob_tool import GlobTool
from .grep_tool import GrepTool


def get_default_tools() -> list[Tool]:
    """Return instances of all built-in tools."""
    return [
        ReadFileTool(),
        WriteFileTool(),
        EditFileTool(),
        ShellTool(),
        GlobTool(),
        GrepTool(),
    ]


def create_default_registry() -> ToolRegistry:
    """Create a ToolRegistry with all built-in tools registered."""
    registry = ToolRegistry()
    for tool in get_default_tools():
        registry.register(tool)
    return registry


__all__ = [
    "Tool",
    "ToolRegistry",
    "ToolResult",
    "ReadFileTool",
    "WriteFileTool",
    "EditFileTool",
    "ShellTool",
    "GlobTool",
    "GrepTool",
    "get_default_tools",
    "create_default_registry",
]
