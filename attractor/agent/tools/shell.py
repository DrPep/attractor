"""Shell command execution tool."""

from __future__ import annotations

from typing import Any, TYPE_CHECKING

from .base import Tool, ToolResult

if TYPE_CHECKING:
    from ..environment import ExecutionEnvironment


class ShellTool(Tool):
    @property
    def name(self) -> str:
        return "shell"

    @property
    def description(self) -> str:
        return "Execute a shell command and return stdout, stderr, and exit code."

    @property
    def parameters_schema(self) -> dict[str, Any]:
        return {
            "type": "object",
            "properties": {
                "command": {
                    "type": "string",
                    "description": "The shell command to execute.",
                },
                "timeout_ms": {
                    "type": "integer",
                    "description": "Timeout in milliseconds (default: 120000).",
                },
            },
            "required": ["command"],
        }

    async def execute(
        self, args: dict[str, Any], env: ExecutionEnvironment,
    ) -> ToolResult:
        command = args.get("command", "")
        timeout_ms = args.get("timeout_ms", 120000)
        timeout_s = timeout_ms / 1000.0

        result = await env.run_command(command, timeout=timeout_s)

        output_parts = []
        if result.stdout:
            output_parts.append(result.stdout)
        if result.stderr:
            output_parts.append(f"STDERR:\n{result.stderr}")
        if result.timed_out:
            output_parts.append(f"Command timed out after {timeout_s}s")

        output = "\n".join(output_parts) if output_parts else "(no output)"

        return ToolResult(
            content=output,
            is_error=result.exit_code != 0,
            metadata={
                "exit_code": result.exit_code,
                "timed_out": result.timed_out,
            },
        )
