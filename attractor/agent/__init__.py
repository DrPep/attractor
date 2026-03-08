"""Coding Agent Loop - programmable agentic loop library."""

from .session import Session, SessionState
from .loop import AgentLoop, SessionConfig, TurnResult
from .events import Event, EventBus, EventType
from .environment import ExecutionEnvironment, CommandResult
from .environments.local import LocalEnvironment
from .history import ConversationHistory, TruncationStrategy
from .loop_detector import LoopDetector
from .steering import SteeringManager
from .provider_profile import ProviderProfile
from .tools import Tool, ToolRegistry, ToolResult, create_default_registry

__all__ = [
    "AgentLoop",
    "CommandResult",
    "ConversationHistory",
    "Event",
    "EventBus",
    "EventType",
    "ExecutionEnvironment",
    "LocalEnvironment",
    "LoopDetector",
    "ProviderProfile",
    "Session",
    "SessionConfig",
    "SessionState",
    "SteeringManager",
    "Tool",
    "ToolRegistry",
    "ToolResult",
    "TruncationStrategy",
    "TurnResult",
    "create_default_registry",
]
