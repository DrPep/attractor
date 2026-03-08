"""Tests for pipeline context."""

from attractor.pipeline.context import PipelineContext


def test_basic_get_set():
    ctx = PipelineContext()
    ctx.set("key", "value")
    assert ctx.get("key") == "value"


def test_dotted_keys():
    ctx = PipelineContext()
    ctx.set("step1.result.status", "success")
    assert ctx.get("step1.result.status") == "success"
    assert ctx.get("step1.result") == {"status": "success"}


def test_default_value():
    ctx = PipelineContext()
    assert ctx.get("missing", "default") == "default"


def test_update():
    ctx = PipelineContext()
    ctx.update({"a": 1, "b": 2})
    assert ctx.get("a") == 1
    assert ctx.get("b") == 2


def test_delete():
    ctx = PipelineContext()
    ctx.set("key", "value")
    ctx.delete("key")
    assert ctx.get("key") is None


def test_snapshot_restore():
    ctx = PipelineContext({"initial": "data"})
    snap = ctx.snapshot()
    ctx.set("new", "value")
    ctx.restore(snap)
    assert ctx.get("new") is None
    assert ctx.get("initial") == "data"


def test_contains():
    ctx = PipelineContext({"key": "val"})
    assert "key" in ctx
    assert "missing" not in ctx


def test_initial_data():
    ctx = PipelineContext({"a": 1, "b": {"c": 2}})
    assert ctx.get("a") == 1
    assert ctx.get("b.c") == 2
