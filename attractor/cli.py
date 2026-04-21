"""CLI entry point for Attractor."""

from __future__ import annotations

import argparse
import asyncio
import functools
import logging
import sys
import uuid
from pathlib import Path
from typing import Any, Callable

from . import __version__


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        prog="attractor",
        description="DOT-based directed graph pipeline runner for multi-stage AI workflows.",
    )
    parser.add_argument("--version", action="version", version=f"%(prog)s {__version__}")
    parser.add_argument(
        "-v", "--verbose", action="count", default=0,
        help="Increase verbosity (-v info, -vv debug)",
    )

    sub = parser.add_subparsers(dest="command")

    # --- run ---
    run_p = sub.add_parser("run", help="Execute a pipeline from a DOT file")
    run_p.add_argument("dotfile", type=Path, help="Path to .dot pipeline file")
    run_p.add_argument("--run-dir", type=Path, default=None, help="Directory for run artifacts")
    run_p.add_argument("--resume", action="store_true", help="Resume from last checkpoint")
    run_p.add_argument("--model", default=None, help="Override LLM model (e.g. claude-opus-4-7)")
    run_p.add_argument("--provider", default=None, help="Override LLM provider (openai, anthropic, gemini)")
    run_p.add_argument("--skills-dir", type=Path, default=None, help="Directory to load skills from")
    run_p.add_argument("--web", action="store_true", help="Launch web UI on --web-port during the run")
    run_p.add_argument("--web-port", type=int, default=8765, help="Port for --web UI (default 8765)")
    run_p.add_argument("--web-host", default="127.0.0.1", help="Host for --web UI (default 127.0.0.1)")

    # --- serve ---
    serve_p = sub.add_parser("serve", help="Launch the web UI over an existing runs/ directory")
    serve_p.add_argument("--runs-dir", type=Path, default=Path("runs"), help="Directory of prior runs")
    serve_p.add_argument("--port", type=int, default=8765, help="Port (default 8765)")
    serve_p.add_argument("--host", default="127.0.0.1", help="Host (default 127.0.0.1)")

    # --- validate ---
    val_p = sub.add_parser("validate", help="Validate a DOT pipeline without executing")
    val_p.add_argument("dotfile", type=Path, help="Path to .dot pipeline file")

    # --- chat ---
    chat_p = sub.add_parser("chat", help="Start an interactive agent session")
    chat_p.add_argument("--model", default=None, help="Override LLM model (e.g. claude-opus-4-7)")
    chat_p.add_argument("--provider", default=None, help="Override LLM provider (openai, anthropic, gemini)")

    args = parser.parse_args(argv)

    level = {0: logging.WARNING, 1: logging.INFO}.get(args.verbose, logging.DEBUG)
    logging.basicConfig(
        level=level,
        format="%(asctime)s %(levelname)-7s %(name)s  %(message)s",
        datefmt="%H:%M:%S",
    )

    if args.command is None:
        parser.print_help()
        return 1

    if args.command == "run":
        return asyncio.run(_cmd_run(args))
    if args.command == "validate":
        return _cmd_validate(args)
    if args.command == "chat":
        return asyncio.run(_cmd_chat(args))
    if args.command == "serve":
        return _cmd_serve(args)

    return 0


# ── run ──────────────────────────────────────────────────────────────────

async def _cmd_run(args: argparse.Namespace) -> int:
    from .llm.client import Client
    from .pipeline.runner import PipelineRunner

    dotfile: Path = args.dotfile
    if not dotfile.exists():
        print(f"Error: file not found: {dotfile}", file=sys.stderr)
        return 1

    client = Client.from_env()
    if not client._providers:
        print(
            "Error: no LLM provider configured. "
            "Set ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY.",
            file=sys.stderr,
        )
        return 1

    # Load skills if a directory is specified
    skill_registry = None
    if args.skills_dir:
        from .agent.skill import SkillRegistry
        skill_registry = SkillRegistry()
        skill_registry.load_dir(args.skills_dir)

    from .pipeline.progress import ProgressTracker

    tracker = ProgressTracker()

    dot_source = dotfile.read_text(encoding="utf-8")

    # If --web is set, bring up the UI server and bridge runner callbacks into it.
    web_ctx = await _start_web_for_run(args, dot_source) if args.web else None

    callbacks: dict[str, Any] = {
        "on_node_start": tracker.on_node_start,
        "on_node_end": tracker.on_node_end,
        "on_edge": tracker.on_edge,
        "on_retry": tracker.on_retry,
        "on_agent_event": tracker.on_agent_event,
    }
    if web_ctx is not None:
        callbacks = _compose_callbacks(callbacks, web_ctx["bridge"])

    runner = PipelineRunner(
        client=client,
        skill_registry=skill_registry,
        model_override=args.model,
        provider_override=args.provider,
        **callbacks,
    )

    run_id = web_ctx["run_id"] if web_ctx else None
    result = await runner.run(
        dot_source, run_dir=args.run_dir, resume=args.resume, run_id=run_id,
    )
    tracker.stop()

    if web_ctx is not None:
        hub = web_ctx["hub"]
        # Persist the DOT inside the run dir so `attractor serve` can find it later.
        try:
            (Path(result.run_dir) / "pipeline.dot").write_text(dot_source, encoding="utf-8")
        except Exception:
            pass
        hub.mark_finished(result.run_id)

    print()
    if result.success:
        print(f"Pipeline succeeded  (run {result.run_id})")
    else:
        print(f"Pipeline failed  (run {result.run_id})")
        for err in result.errors:
            print(f"  error: {err}", file=sys.stderr)

    print(f"  nodes executed: {', '.join(result.nodes_executed)}")
    print(f"  run dir: {result.run_dir}")

    if web_ctx is not None:
        print(f"\n  web UI is still serving on http://{args.web_host}:{args.web_port}/")
        print("  Press Ctrl-C to exit.")
        try:
            await web_ctx["serve_task"]
        except (KeyboardInterrupt, asyncio.CancelledError):
            pass

    return 0 if result.success else 1


# ── validate ─────────────────────────────────────────────────────────────

def _cmd_validate(args: argparse.Namespace) -> int:
    from .pipeline.parser import parse_dot
    from .pipeline.validation import validate, Severity

    dotfile: Path = args.dotfile
    if not dotfile.exists():
        print(f"Error: file not found: {dotfile}", file=sys.stderr)
        return 1

    dot_source = dotfile.read_text(encoding="utf-8")

    try:
        graph = parse_dot(dot_source)
    except Exception as e:
        print(f"Parse error: {e}", file=sys.stderr)
        return 1

    diagnostics = validate(graph)

    errors = [d for d in diagnostics if d.severity == Severity.ERROR]
    warnings = [d for d in diagnostics if d.severity == Severity.WARNING]

    for d in errors:
        print(f"  ERROR   {d.message}")
        if d.suggested_fix:
            print(f"          fix: {d.suggested_fix}")

    for d in warnings:
        print(f"  WARN    {d.message}")
        if d.suggested_fix:
            print(f"          fix: {d.suggested_fix}")

    if errors:
        print(f"\n{len(errors)} error(s), {len(warnings)} warning(s)")
        return 1

    if warnings:
        print(f"\nValid with {len(warnings)} warning(s)")
    else:
        print("Valid")

    return 0


# ── chat ─────────────────────────────────────────────────────────────────

async def _cmd_chat(args: argparse.Namespace) -> int:
    from .agent.session import Session
    from .agent.loop import SessionConfig
    from .agent.provider_profile import ProviderProfile
    from .llm.client import Client

    client = Client.from_env()
    if not client._providers:
        print(
            "Error: no LLM provider configured. "
            "Set ANTHROPIC_API_KEY, OPENAI_API_KEY, or GEMINI_API_KEY.",
            file=sys.stderr,
        )
        return 1

    profile = None
    config = None
    if args.model or args.provider:
        model = args.model or "claude-opus-4-7"
        provider = args.provider or "anthropic"
        profile = ProviderProfile(provider_name=provider, model=model)
        config = SessionConfig(model=model, provider=provider)

    session = Session(client=client, profile=profile, config=config)

    print(f"Attractor agent  (model: {session.config.model})")
    print("Type /quit to exit.\n")

    while True:
        try:
            user_input = input("you> ").strip()
        except (EOFError, KeyboardInterrupt):
            print()
            break

        if not user_input:
            continue
        if user_input in ("/quit", "/exit"):
            break

        try:
            result = await session.submit(user_input)
            print(f"\nagent> {result.final_response}\n")
        except Exception as e:
            print(f"\nerror: {e}\n", file=sys.stderr)

    session.close()
    return 0


# ── web ──────────────────────────────────────────────────────────────────

def _require_web_extra() -> None:
    try:
        import fastapi  # noqa: F401
        import uvicorn  # noqa: F401
        import sse_starlette  # noqa: F401
    except ImportError as e:
        print(
            f"Error: web UI requires the [web] extra. Install with:\n"
            f"    pip install 'attractor[web]'\n"
            f"(missing: {e.name})",
            file=sys.stderr,
        )
        raise SystemExit(1)


async def _start_web_for_run(args: argparse.Namespace, dot_source: str) -> dict[str, Any]:
    """Spin up the UI server in the background for a run triggered by `run --web`."""
    _require_web_extra()
    import uvicorn

    from .web.events import EventHub
    from .web.runner_bridge import attach_to_runner
    from .web.server import create_app

    run_id = uuid.uuid4().hex[:12]
    runs_dir = (args.run_dir.parent if args.run_dir else Path("runs")).resolve()
    hub = EventHub()
    hub.set_graph(run_id, dot_source)

    app = create_app(runs_dir=runs_dir, hub=hub)
    config = uvicorn.Config(
        app, host=args.web_host, port=args.web_port, log_level="warning", lifespan="on",
    )
    server = uvicorn.Server(config)
    serve_task = asyncio.create_task(server.serve())
    # Give uvicorn a moment to bind before the run emits its first events.
    for _ in range(40):
        if server.started:
            break
        await asyncio.sleep(0.05)

    print(f"  web UI: http://{args.web_host}:{args.web_port}/?run={run_id}")

    bridge = attach_to_runner(hub, run_id)
    return {
        "hub": hub,
        "bridge": bridge,
        "run_id": run_id,
        "server": server,
        "serve_task": serve_task,
    }


def _compose_callbacks(
    base: dict[str, Any], extra: dict[str, Any]
) -> dict[str, Any]:
    """Return callbacks that call both `base[k]` and `extra[k]`."""
    out: dict[str, Any] = {}
    for key in base:
        b = base[key]
        e = extra.get(key)
        if b is None:
            out[key] = e
        elif e is None:
            out[key] = b
        else:
            out[key] = _chain(b, e)
    return out


def _chain(f: Callable[..., Any], g: Callable[..., Any]) -> Callable[..., Any]:
    # Preserve f's signature so runner.py's arity probe (inspect.signature)
    # sees the intended shape rather than the generic *a/**kw of the wrapper.
    @functools.wraps(f)
    def _both(*a: Any, **kw: Any) -> None:
        try:
            f(*a, **kw)
        finally:
            g(*a, **kw)
    return _both


def _cmd_serve(args: argparse.Namespace) -> int:
    _require_web_extra()
    import uvicorn

    from .web.events import EventHub
    from .web.server import create_app

    runs_dir = args.runs_dir.resolve()
    app = create_app(runs_dir=runs_dir, hub=EventHub())
    print(f"Attractor UI: http://{args.host}:{args.port}/  (serving {runs_dir})")
    uvicorn.run(app, host=args.host, port=args.port, log_level="warning")
    return 0


if __name__ == "__main__":
    sys.exit(main())
