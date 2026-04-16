"""CLI entry point for Attractor."""

from __future__ import annotations

import argparse
import asyncio
import logging
import sys
from pathlib import Path

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
    run_p.add_argument("--model", default=None, help="Override LLM model (e.g. claude-sonnet-4-6)")
    run_p.add_argument("--provider", default=None, help="Override LLM provider (openai, anthropic, gemini)")
    run_p.add_argument("--skills-dir", type=Path, default=None, help="Directory to load skills from")

    # --- validate ---
    val_p = sub.add_parser("validate", help="Validate a DOT pipeline without executing")
    val_p.add_argument("dotfile", type=Path, help="Path to .dot pipeline file")

    # --- chat ---
    chat_p = sub.add_parser("chat", help="Start an interactive agent session")
    chat_p.add_argument("--model", default=None, help="Override LLM model (e.g. claude-sonnet-4-6)")
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

    runner = PipelineRunner(
        client=client,
        skill_registry=skill_registry,
        model_override=args.model,
        provider_override=args.provider,
        on_node_start=tracker.on_node_start,
        on_node_end=tracker.on_node_end,
        on_edge=tracker.on_edge,
        on_retry=tracker.on_retry,
        on_agent_event=tracker.on_agent_event,
    )

    dot_source = dotfile.read_text(encoding="utf-8")
    result = await runner.run(dot_source, run_dir=args.run_dir, resume=args.resume)
    tracker.stop()

    print()
    if result.success:
        print(f"Pipeline succeeded  (run {result.run_id})")
    else:
        print(f"Pipeline failed  (run {result.run_id})")
        for err in result.errors:
            print(f"  error: {err}", file=sys.stderr)

    print(f"  nodes executed: {', '.join(result.nodes_executed)}")
    print(f"  run dir: {result.run_dir}")

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
        model = args.model or "claude-sonnet-4-6"
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


if __name__ == "__main__":
    sys.exit(main())
