"""Optional web UI for visualizing pipeline runs.

Install with `pip install attractor[web]`. Import lazily — depends on fastapi/uvicorn.
"""

from __future__ import annotations

__all__ = ["create_app", "EventHub", "attach_to_runner"]


def __getattr__(name: str):  # lazy re-exports so `import attractor.web` doesn't pull fastapi
    if name == "create_app":
        from .server import create_app
        return create_app
    if name == "EventHub":
        from .events import EventHub
        return EventHub
    if name == "attach_to_runner":
        from .runner_bridge import attach_to_runner
        return attach_to_runner
    raise AttributeError(name)
