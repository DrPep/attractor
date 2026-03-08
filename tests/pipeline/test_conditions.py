"""Tests for condition expression language."""

from attractor.pipeline.conditions import evaluate, validate_condition
from attractor.pipeline.context import PipelineContext


def test_empty_condition():
    ctx = PipelineContext()
    assert evaluate("", ctx) is True


def test_outcome_equals():
    ctx = PipelineContext()
    assert evaluate("outcome = success", ctx, outcome="success")
    assert not evaluate("outcome = success", ctx, outcome="fail")


def test_outcome_not_equals():
    ctx = PipelineContext()
    assert evaluate("outcome != fail", ctx, outcome="success")
    assert not evaluate("outcome != fail", ctx, outcome="fail")


def test_preferred_label():
    ctx = PipelineContext()
    assert evaluate("preferred_label = Fix", ctx, preferred_label="Fix")
    assert not evaluate("preferred_label = Fix", ctx, preferred_label="Skip")


def test_context_variable():
    ctx = PipelineContext({"tests_passed": "true"})
    assert evaluate("context.tests_passed = true", ctx)


def test_and_conjunction():
    ctx = PipelineContext({"tests_passed": "true"})
    assert evaluate(
        "outcome = success && context.tests_passed = true",
        ctx, outcome="success",
    )
    assert not evaluate(
        "outcome = success && context.tests_passed = true",
        ctx, outcome="fail",
    )


def test_missing_context_is_empty():
    ctx = PipelineContext()
    assert evaluate("context.missing = ", ctx)
    assert not evaluate("context.missing = something", ctx)


def test_validate_condition_valid():
    errors = validate_condition("outcome = success")
    assert len(errors) == 0


def test_validate_condition_invalid():
    errors = validate_condition("invalid clause without operator")
    assert len(errors) > 0
