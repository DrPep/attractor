"""Status file management and run directory structure."""

from __future__ import annotations

import json
import os
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field


class NodeStatus(BaseModel):
    """Status of a completed node."""
    outcome: str = "success"  # success, retry, fail, partial_success
    preferred_next_label: str = ""
    suggested_next_ids: list[str] = Field(default_factory=list)
    context_updates: dict[str, Any] = Field(default_factory=dict)
    notes: str = ""


def write_status(node_id: str, status: NodeStatus, run_dir: Path) -> None:
    """Write status.json for a node."""
    node_dir = run_dir / node_id
    node_dir.mkdir(parents=True, exist_ok=True)
    target = node_dir / "status.json"
    target.write_text(status.model_dump_json(indent=2), encoding="utf-8")


def read_status(node_id: str, run_dir: Path) -> NodeStatus | None:
    """Read status.json for a node."""
    target = run_dir / node_id / "status.json"
    if not target.exists():
        return None
    try:
        data = json.loads(target.read_text(encoding="utf-8"))
        return NodeStatus(**data)
    except Exception:
        return None


def write_prompt(node_id: str, prompt: str, run_dir: Path) -> None:
    """Write prompt.md for a node."""
    node_dir = run_dir / node_id
    node_dir.mkdir(parents=True, exist_ok=True)
    (node_dir / "prompt.md").write_text(prompt, encoding="utf-8")


def write_response(node_id: str, response: str, run_dir: Path) -> None:
    """Write response.md for a node."""
    node_dir = run_dir / node_id
    node_dir.mkdir(parents=True, exist_ok=True)
    (node_dir / "response.md").write_text(response, encoding="utf-8")


def ensure_run_dir(run_dir: Path) -> None:
    """Ensure the run directory structure exists."""
    run_dir.mkdir(parents=True, exist_ok=True)
    (run_dir / "artifacts").mkdir(exist_ok=True)
