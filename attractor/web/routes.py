"""REST + SSE routes for the Attractor web UI."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any

from fastapi import APIRouter, HTTPException, Request
from sse_starlette.sse import EventSourceResponse

from .events import EventHub


def build_router() -> APIRouter:
    router = APIRouter()

    @router.get("/runs")
    def list_runs(request: Request) -> dict[str, Any]:
        runs_dir: Path = request.app.state.runs_dir
        hub: EventHub = request.app.state.hub
        disk_runs = _list_disk_runs(runs_dir)
        live_runs = set(hub.list_runs())
        all_ids = set(disk_runs) | live_runs
        # Live runs first, then disk runs reverse-sorted so newest-ish show up early.
        ordered = sorted(
            all_ids,
            key=lambda r: (0 if r in live_runs else 1, r if r in live_runs else ""),
        )
        ordered_live = [r for r in ordered if r in live_runs]
        ordered_disk = sorted((r for r in all_ids if r not in live_runs), reverse=True)
        runs = [
            {"run_id": r, "live": True, "on_disk": r in disk_runs}
            for r in ordered_live
        ] + [
            {"run_id": r, "live": False, "on_disk": True}
            for r in ordered_disk
        ]
        return {"runs": runs}

    @router.get("/runs/{run_id}")
    def get_run(run_id: str, request: Request) -> dict[str, Any]:
        runs_dir: Path = request.app.state.runs_dir
        hub: EventHub = request.app.state.hub

        snapshot = hub.snapshot(run_id)
        run_dir = runs_dir / run_id

        graph_dot = snapshot.get("graph_dot") or _read_graph_dot(run_dir)
        node_state = dict(snapshot.get("node_state") or {})

        # Fold in on-disk statuses for any nodes not already tracked live.
        for nid, status in _read_disk_statuses(run_dir).items():
            entry = node_state.setdefault(nid, {})
            entry.setdefault("status", status)

        if not graph_dot and not node_state:
            raise HTTPException(status_code=404, detail=f"unknown run {run_id}")

        return {
            "run_id": run_id,
            "live": snapshot.get("known", False) and not snapshot.get("finished", False),
            "finished": snapshot.get("finished", False),
            "graph_dot": graph_dot,
            "node_state": node_state,
            "next_seq": snapshot.get("next_seq", 0),
        }

    @router.get("/runs/{run_id}/nodes/{node_id}")
    def get_node(run_id: str, node_id: str, request: Request) -> dict[str, Any]:
        runs_dir: Path = request.app.state.runs_dir
        node_dir = runs_dir / run_id / node_id
        if not node_dir.exists():
            raise HTTPException(status_code=404, detail=f"no artifacts for {node_id}")
        return {
            "node_id": node_id,
            "prompt": _read_text(node_dir / "prompt.md"),
            "response": _read_text(node_dir / "response.md"),
            "status": _read_json(node_dir / "status.json"),
        }

    @router.get("/runs/{run_id}/events")
    async def stream_events(run_id: str, request: Request, since: int = 0):
        hub: EventHub = request.app.state.hub

        async def gen():
            async for ev in hub.subscribe(run_id, since_seq=since):
                if await request.is_disconnected():
                    break
                yield {
                    "event": ev.kind,
                    "id": str(ev.seq),
                    "data": json.dumps(ev.to_dict()),
                }

        return EventSourceResponse(gen())

    return router


# ── disk helpers ───────────────────────────────────────────────────────────

def _list_disk_runs(runs_dir: Path) -> list[str]:
    if not runs_dir.exists():
        return []
    return [p.name for p in runs_dir.iterdir() if p.is_dir()]


def _read_graph_dot(run_dir: Path) -> str:
    # The runner writes the source DOT alongside artifacts. Check a couple of
    # likely locations; fall back to empty string if not found.
    for candidate in ("pipeline.dot", "graph.dot", "source.dot"):
        p = run_dir / candidate
        if p.exists():
            return p.read_text(encoding="utf-8")
    return ""


def _read_disk_statuses(run_dir: Path) -> dict[str, str]:
    out: dict[str, str] = {}
    if not run_dir.exists():
        return out
    for sub in run_dir.iterdir():
        if not sub.is_dir():
            continue
        status_file = sub / "status.json"
        if status_file.exists():
            try:
                data = json.loads(status_file.read_text(encoding="utf-8"))
                out[sub.name] = str(data.get("outcome", "done"))
            except Exception:
                pass
    return out


def _read_text(path: Path) -> str:
    if not path.exists():
        return ""
    try:
        return path.read_text(encoding="utf-8")
    except Exception:
        return ""


def _read_json(path: Path) -> dict[str, Any]:
    if not path.exists():
        return {}
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except Exception:
        return {}
