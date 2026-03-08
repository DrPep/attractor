"""Checkpoint save/restore for pipeline state."""

from __future__ import annotations

import json
import os
import tempfile
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field


class Checkpoint(BaseModel):
    timestamp: str = ""
    run_id: str = ""
    current_node: str = ""
    completed_nodes: list[str] = Field(default_factory=list)
    node_retries: dict[str, int] = Field(default_factory=dict)
    context_snapshot: dict[str, Any] = Field(default_factory=dict)

    def model_post_init(self, __context: Any) -> None:
        if not self.timestamp:
            self.timestamp = datetime.now(timezone.utc).isoformat()


def save_checkpoint(checkpoint: Checkpoint, run_dir: Path) -> None:
    """Save checkpoint atomically (write temp, rename)."""
    run_dir.mkdir(parents=True, exist_ok=True)
    target = run_dir / "checkpoint.json"

    # Atomic write
    fd, tmp_path = tempfile.mkstemp(dir=str(run_dir), suffix=".tmp")
    try:
        with os.fdopen(fd, "w") as f:
            f.write(checkpoint.model_dump_json(indent=2))
        os.replace(tmp_path, str(target))
    except Exception:
        if os.path.exists(tmp_path):
            os.unlink(tmp_path)
        raise


def load_checkpoint(run_dir: Path) -> Checkpoint | None:
    """Load checkpoint from run directory."""
    target = run_dir / "checkpoint.json"
    if not target.exists():
        return None
    try:
        data = json.loads(target.read_text(encoding="utf-8"))
        return Checkpoint(**data)
    except Exception:
        return None
