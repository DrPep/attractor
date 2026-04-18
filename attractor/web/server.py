"""FastAPI app factory for the Attractor web UI."""

from __future__ import annotations

from pathlib import Path

from fastapi import FastAPI
from fastapi.staticfiles import StaticFiles

from .events import EventHub
from .routes import build_router


def create_app(runs_dir: Path, hub: EventHub | None = None) -> FastAPI:
    """Build the FastAPI app.

    runs_dir — directory scanned for historical runs (matches PipelineRunner layout).
    hub      — optional shared EventHub so a running pipeline can stream into this app.
    """
    app = FastAPI(title="Attractor", docs_url="/api/docs", openapi_url="/api/openapi.json")
    app.state.runs_dir = Path(runs_dir)
    app.state.hub = hub or EventHub()

    app.include_router(build_router(), prefix="/api")

    static_dir = Path(__file__).parent / "static"
    app.mount("/", StaticFiles(directory=str(static_dir), html=True), name="static")

    return app
