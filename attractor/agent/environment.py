"""Execution environment protocol."""

from __future__ import annotations

from abc import ABC, abstractmethod
from pathlib import Path

from pydantic import BaseModel


class CommandResult(BaseModel):
    stdout: str = ""
    stderr: str = ""
    exit_code: int = 0
    timed_out: bool = False


class ExecutionEnvironment(ABC):
    """Where tools execute: local, Docker, SSH, etc."""

    @property
    @abstractmethod
    def working_directory(self) -> Path: ...

    @abstractmethod
    async def run_command(
        self, command: str, timeout: float = 120.0,
        cwd: str | None = None,
    ) -> CommandResult: ...

    @abstractmethod
    async def read_file(self, path: str) -> str: ...

    @abstractmethod
    async def write_file(self, path: str, content: str) -> None: ...

    @abstractmethod
    async def list_files(
        self, pattern: str, base_path: str | None = None,
    ) -> list[str]: ...

    @abstractmethod
    async def file_exists(self, path: str) -> bool: ...
