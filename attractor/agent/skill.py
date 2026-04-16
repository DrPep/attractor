"""Skill definitions: composable prompt + tool set modifiers for agent nodes."""

from __future__ import annotations

import importlib.util
import logging
from pathlib import Path
from typing import Any

from pydantic import BaseModel, Field

from .tools.base import Tool, ToolRegistry
from .tools import create_default_registry

logger = logging.getLogger(__name__)


class Skill(BaseModel):
    """A reusable bundle of system prompt additions and tool set modifications."""

    name: str
    description: str = ""
    system_prompt: str = ""
    tools_include: list[str] = Field(default_factory=list)
    tools_exclude: list[str] = Field(default_factory=list)

    model_config = {"arbitrary_types_allowed": True}


class ComposedSkill(BaseModel):
    """Flattened result of merging multiple skills."""

    system_prompt: str = ""
    tools_exclude: list[str] = Field(default_factory=list)
    extra_tools: list[Any] = Field(default_factory=list)

    model_config = {"arbitrary_types_allowed": True}


class SkillRegistry:
    """Loads and composes skills from files and programmatic registration."""

    def __init__(self) -> None:
        self._skills: dict[str, Skill] = {}
        self._custom_tools: dict[str, list[Tool]] = {}  # skill name -> custom tools

    def register(self, skill: Skill, custom_tools: list[Tool] | None = None) -> None:
        """Register a skill programmatically."""
        self._skills[skill.name] = skill
        if custom_tools:
            self._custom_tools[skill.name] = custom_tools

    def get(self, name: str) -> Skill | None:
        return self._skills.get(name)

    def list_skills(self) -> list[Skill]:
        return list(self._skills.values())

    def load_dir(self, path: Path) -> None:
        """Load skills from a directory. Supports .yaml/.yml and .py files."""
        if not path.is_dir():
            logger.warning("Skill directory does not exist: %s", path)
            return

        for entry in sorted(path.iterdir()):
            if entry.suffix in (".yaml", ".yml"):
                self._load_yaml(entry)
            elif entry.suffix == ".py" and not entry.name.startswith("_"):
                self._load_python(entry)

    def _load_yaml(self, path: Path) -> None:
        """Load a skill from a YAML file."""
        try:
            import yaml
        except ImportError:
            logger.warning(
                "PyYAML not installed, skipping skill file: %s. "
                "Install with: pip install pyyaml",
                path,
            )
            return

        try:
            data = yaml.safe_load(path.read_text(encoding="utf-8"))
            if not isinstance(data, dict):
                logger.warning("Skill file %s does not contain a mapping", path)
                return
            skill = Skill(**data)
            self._skills[skill.name] = skill
            logger.debug("Loaded skill %r from %s", skill.name, path)
        except Exception as e:
            logger.warning("Failed to load skill from %s: %s", path, e)

    def _load_python(self, path: Path) -> None:
        """Load a skill from a Python module. Expects a module-level `skill` attribute."""
        try:
            spec = importlib.util.spec_from_file_location(f"skill_{path.stem}", path)
            if not spec or not spec.loader:
                logger.warning("Could not load module spec from %s", path)
                return
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)

            skill = getattr(module, "skill", None)
            if not isinstance(skill, Skill):
                logger.warning("Module %s has no 'skill' attribute of type Skill", path)
                return

            custom_tools = getattr(module, "tools", None)
            if custom_tools and not isinstance(custom_tools, list):
                custom_tools = None

            self.register(skill, custom_tools=custom_tools)
            logger.debug("Loaded skill %r from %s", skill.name, path)
        except Exception as e:
            logger.warning("Failed to load skill from %s: %s", path, e)

    def compose(self, names: list[str]) -> ComposedSkill:
        """Merge multiple skills by name into a single ComposedSkill.

        System prompts are concatenated with newlines. Tool excludes are unioned.
        Custom tools from all referenced skills are collected.
        Unknown skill names are logged as warnings and skipped.
        """
        prompt_parts: list[str] = []
        all_excludes: list[str] = []
        all_extra_tools: list[Tool] = []

        for name in names:
            skill = self._skills.get(name)
            if not skill:
                logger.warning("Unknown skill referenced: %r", name)
                continue

            if skill.system_prompt:
                prompt_parts.append(skill.system_prompt.strip())
            all_excludes.extend(skill.tools_exclude)

            custom = self._custom_tools.get(name, [])
            all_extra_tools.extend(custom)

        return ComposedSkill(
            system_prompt="\n\n".join(prompt_parts),
            tools_exclude=list(dict.fromkeys(all_excludes)),  # dedupe, preserve order
            extra_tools=all_extra_tools,
        )

    def build_tool_registry(self, composed: ComposedSkill) -> ToolRegistry:
        """Build a ToolRegistry from defaults, applying skill modifications."""
        registry = create_default_registry()
        for name in composed.tools_exclude:
            registry.remove(name)
        for tool in composed.extra_tools:
            registry.register(tool)
        return registry
