"""Tests for skill registry, composition, and loading."""

from __future__ import annotations

from pathlib import Path

import pytest

from attractor.agent.skill import ComposedSkill, Skill, SkillRegistry
from attractor.agent.tools.base import Tool, ToolRegistry, ToolResult


# ── helpers ──────────────────────────────────────────────────────────────


class DummyTool(Tool):
    def __init__(self, tool_name: str):
        self._name = tool_name

    @property
    def name(self) -> str:
        return self._name

    @property
    def description(self) -> str:
        return f"Dummy tool: {self._name}"

    @property
    def parameters_schema(self) -> dict:
        return {"type": "object", "properties": {}}

    async def execute(self, args, env) -> ToolResult:
        return ToolResult(content="ok")


# ── Skill model ──────────────────────────────────────────────────────────


def test_skill_defaults():
    s = Skill(name="test")
    assert s.name == "test"
    assert s.system_prompt == ""
    assert s.tools_include == []
    assert s.tools_exclude == []


def test_skill_with_values():
    s = Skill(
        name="review",
        description="Code review",
        system_prompt="You are reviewing code.",
        tools_exclude=["write_file", "edit_file"],
    )
    assert s.tools_exclude == ["write_file", "edit_file"]
    assert "reviewing" in s.system_prompt


# ── SkillRegistry ────────────────────────────────────────────────────────


def test_register_and_get():
    reg = SkillRegistry()
    skill = Skill(name="alpha")
    reg.register(skill)
    assert reg.get("alpha") is skill
    assert reg.get("nonexistent") is None


def test_list_skills():
    reg = SkillRegistry()
    reg.register(Skill(name="a"))
    reg.register(Skill(name="b"))
    names = {s.name for s in reg.list_skills()}
    assert names == {"a", "b"}


def test_register_with_custom_tools():
    reg = SkillRegistry()
    tool = DummyTool("deploy")
    skill = Skill(name="deployer", system_prompt="Deploy things.")
    reg.register(skill, custom_tools=[tool])

    composed = reg.compose(["deployer"])
    assert len(composed.extra_tools) == 1
    assert composed.extra_tools[0].name == "deploy"


# ── compose ──────────────────────────────────────────────────────────────


def test_compose_empty():
    reg = SkillRegistry()
    composed = reg.compose([])
    assert composed.system_prompt == ""
    assert composed.tools_exclude == []
    assert composed.extra_tools == []


def test_compose_single():
    reg = SkillRegistry()
    reg.register(Skill(
        name="review",
        system_prompt="Review carefully.",
        tools_exclude=["write_file"],
    ))
    composed = reg.compose(["review"])
    assert composed.system_prompt == "Review carefully."
    assert composed.tools_exclude == ["write_file"]


def test_compose_multiple_merges_prompts():
    reg = SkillRegistry()
    reg.register(Skill(name="a", system_prompt="Prompt A."))
    reg.register(Skill(name="b", system_prompt="Prompt B."))
    composed = reg.compose(["a", "b"])
    assert "Prompt A." in composed.system_prompt
    assert "Prompt B." in composed.system_prompt
    # Separated by double newline
    assert "\n\n" in composed.system_prompt


def test_compose_deduplicates_excludes():
    reg = SkillRegistry()
    reg.register(Skill(name="a", tools_exclude=["write_file", "edit_file"]))
    reg.register(Skill(name="b", tools_exclude=["edit_file", "shell"]))
    composed = reg.compose(["a", "b"])
    assert composed.tools_exclude == ["write_file", "edit_file", "shell"]


def test_compose_unknown_skill_skipped():
    reg = SkillRegistry()
    reg.register(Skill(name="known", system_prompt="Hello."))
    composed = reg.compose(["known", "unknown"])
    assert composed.system_prompt == "Hello."


# ── build_tool_registry ──────────────────────────────────────────────────


def test_build_tool_registry_excludes():
    reg = SkillRegistry()
    reg.register(Skill(name="readonly", tools_exclude=["write_file", "edit_file"]))
    composed = reg.compose(["readonly"])
    tool_reg = reg.build_tool_registry(composed)
    assert tool_reg.get("write_file") is None
    assert tool_reg.get("edit_file") is None
    # Other tools should still be present
    assert tool_reg.get("read_file") is not None


def test_build_tool_registry_adds_custom_tools():
    reg = SkillRegistry()
    custom = DummyTool("custom_tool")
    reg.register(Skill(name="custom"), custom_tools=[custom])
    composed = reg.compose(["custom"])
    tool_reg = reg.build_tool_registry(composed)
    assert tool_reg.get("custom_tool") is not None
    assert tool_reg.get("custom_tool").name == "custom_tool"


# ── ToolRegistry.remove ──────────────────────────────────────────────────


def test_tool_registry_remove():
    reg = ToolRegistry()
    reg.register(DummyTool("foo"))
    reg.register(DummyTool("bar"))
    assert reg.get("foo") is not None
    reg.remove("foo")
    assert reg.get("foo") is None
    assert reg.get("bar") is not None


def test_tool_registry_remove_nonexistent():
    reg = ToolRegistry()
    reg.remove("nonexistent")  # should not raise


# ── YAML loading ─────────────────────────────────────────────────────────


def test_load_yaml_skill(tmp_path):
    try:
        import yaml
    except ImportError:
        pytest.skip("pyyaml not installed")

    skill_file = tmp_path / "review.yaml"
    skill_file.write_text(
        "name: code-review\n"
        "description: Code review skill\n"
        "system_prompt: Review the code carefully.\n"
        "tools_exclude:\n"
        "  - write_file\n"
        "  - edit_file\n"
    )

    reg = SkillRegistry()
    reg.load_dir(tmp_path)
    skill = reg.get("code-review")
    assert skill is not None
    assert skill.description == "Code review skill"
    assert skill.tools_exclude == ["write_file", "edit_file"]


# ── Python module loading ────────────────────────────────────────────────


def test_load_python_skill(tmp_path):
    skill_file = tmp_path / "deploy.py"
    skill_file.write_text(
        "from attractor.agent.skill import Skill\n"
        "skill = Skill(name='deploy', system_prompt='Deploy things.')\n"
    )

    reg = SkillRegistry()
    reg.load_dir(tmp_path)
    skill = reg.get("deploy")
    assert skill is not None
    assert "Deploy" in skill.system_prompt


def test_load_dir_nonexistent(tmp_path):
    reg = SkillRegistry()
    reg.load_dir(tmp_path / "nonexistent")  # should not raise
    assert reg.list_skills() == []


# ── Node.skills property ────────────────────────────────────────────────


def test_node_skills_property():
    from attractor.pipeline.graph import Node
    node = Node(id="test", attrs={"skills": "code-review,testing"})
    assert node.skills == ["code-review", "testing"]


def test_node_skills_empty():
    from attractor.pipeline.graph import Node
    node = Node(id="test", attrs={})
    assert node.skills == []


def test_node_skills_single():
    from attractor.pipeline.graph import Node
    node = Node(id="test", attrs={"skills": "review"})
    assert node.skills == ["review"]


def test_node_skills_with_spaces():
    from attractor.pipeline.graph import Node
    node = Node(id="test", attrs={"skills": " review , testing , deploy "})
    assert node.skills == ["review", "testing", "deploy"]
