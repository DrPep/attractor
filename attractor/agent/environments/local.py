"""Local execution environment using subprocess."""

from __future__ import annotations

import asyncio
import glob as glob_module
from pathlib import Path

from ..environment import CommandResult, ExecutionEnvironment


class LocalEnvironment(ExecutionEnvironment):
    """Executes tools on the local machine."""

    def __init__(self, working_dir: str | Path | None = None):
        self._working_dir = Path(working_dir) if working_dir else Path.cwd()

    @property
    def working_directory(self) -> Path:
        return self._working_dir

    async def run_command(
        self, command: str, timeout: float = 120.0,
        cwd: str | None = None,
    ) -> CommandResult:
        work_dir = cwd or str(self._working_dir)
        try:
            proc = await asyncio.create_subprocess_shell(
                command,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE,
                cwd=work_dir,
            )
            try:
                stdout, stderr = await asyncio.wait_for(
                    proc.communicate(), timeout=timeout,
                )
                return CommandResult(
                    stdout=stdout.decode(errors="replace"),
                    stderr=stderr.decode(errors="replace"),
                    exit_code=proc.returncode or 0,
                )
            except asyncio.TimeoutError:
                proc.kill()
                await proc.communicate()
                return CommandResult(
                    stderr=f"Command timed out after {timeout}s",
                    exit_code=-1,
                    timed_out=True,
                )
        except Exception as e:
            return CommandResult(
                stderr=str(e), exit_code=-1,
            )

    async def read_file(self, path: str) -> str:
        resolved = self._resolve_path(path)
        return await asyncio.to_thread(resolved.read_text, encoding="utf-8")

    async def write_file(self, path: str, content: str) -> None:
        resolved = self._resolve_path(path)
        resolved.parent.mkdir(parents=True, exist_ok=True)
        await asyncio.to_thread(resolved.write_text, content, encoding="utf-8")

    async def list_files(
        self, pattern: str, base_path: str | None = None,
    ) -> list[str]:
        base = Path(base_path) if base_path else self._working_dir
        if not base.is_absolute():
            base = self._working_dir / base

        def _glob() -> list[str]:
            matches = list(base.glob(pattern))
            return sorted(str(m) for m in matches if m.is_file())

        return await asyncio.to_thread(_glob)

    async def file_exists(self, path: str) -> bool:
        resolved = self._resolve_path(path)
        return await asyncio.to_thread(resolved.exists)

    def _resolve_path(self, path: str) -> Path:
        p = Path(path)
        if p.is_absolute():
            return p
        return self._working_dir / p
