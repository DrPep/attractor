"""Tests for model catalog."""

from attractor.llm.catalog import (
    get_model_info, list_models, get_latest_model, resolve_provider,
)


def test_get_model_info():
    info = get_model_info("claude-opus-4-6")
    assert info is not None
    assert info.provider == "anthropic"
    assert info.supports_tools


def test_get_model_info_by_alias():
    info = get_model_info("opus")
    assert info is not None
    assert info.id == "claude-opus-4-6"


def test_get_model_info_unknown():
    assert get_model_info("nonexistent-model") is None


def test_list_models_all():
    models = list_models()
    assert len(models) >= 10


def test_list_models_by_provider():
    anthropic = list_models("anthropic")
    assert all(m.provider == "anthropic" for m in anthropic)
    assert len(anthropic) >= 3


def test_get_latest_model():
    latest = get_latest_model("anthropic")
    assert latest is not None
    assert latest.provider == "anthropic"


def test_get_latest_model_with_capability():
    latest = get_latest_model("anthropic", "reasoning")
    assert latest is not None
    assert latest.supports_reasoning


def test_resolve_provider():
    assert resolve_provider("claude-opus-4-6") == "anthropic"
    assert resolve_provider("gpt-5.2") == "openai"
    assert resolve_provider("gemini-3-pro-preview") == "gemini"
    assert resolve_provider("unknown") is None
