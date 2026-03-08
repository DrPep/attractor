"""Session management with state machine."""

from __future__ import annotations

from enum import Enum
from typing import Any, Callable

from ..llm.client import Client
from ..llm.models import Message
from .environment import ExecutionEnvironment
from .environments.local import LocalEnvironment
from .events import Event, EventBus, EventType
from .history import ConversationHistory
from .loop import AgentLoop, SessionConfig, TurnResult
from .loop_detector import LoopDetector
from .provider_profile import ProviderProfile
from .steering import SteeringManager
from .tools import create_default_registry
from .tools.base import ToolRegistry


class SessionState(str, Enum):
    IDLE = "idle"
    PROCESSING = "processing"
    AWAITING_INPUT = "awaiting_input"
    CLOSED = "closed"


class Session:
    """Main entry point for the coding agent. Manages state, history, and the agent loop."""

    def __init__(
        self,
        client: Client,
        profile: ProviderProfile | None = None,
        environment: ExecutionEnvironment | None = None,
        config: SessionConfig | None = None,
        tool_registry: ToolRegistry | None = None,
    ):
        self._client = client
        self._profile = profile or ProviderProfile.for_anthropic()
        self._env = environment or LocalEnvironment()
        self._config = config or SessionConfig(model=self._profile.model)
        if self._profile.provider_name and not self._config.provider:
            self._config.provider = self._profile.provider_name

        self._state = SessionState.IDLE
        self._history = ConversationHistory()
        self._event_bus = EventBus()
        self._loop_detector = LoopDetector()
        self._steering = SteeringManager()

        # Set up tools
        self._tool_registry = tool_registry or create_default_registry()

        # Register profile-provided tool definitions (already in the registry
        # as actual tools; the profile tools are just extra definitions for LLM)
        if self._profile.tools:
            for td in self._profile.tools:
                if not self._tool_registry.get(td.name):
                    pass  # Only include tools we have implementations for

        # Create agent loop
        self._loop = AgentLoop(
            client=self._client,
            tool_registry=self._tool_registry,
            event_bus=self._event_bus,
            loop_detector=self._loop_detector,
            steering=self._steering,
        )
        self._loop.set_execution_env(self._env)

    @property
    def state(self) -> SessionState:
        return self._state

    @property
    def history(self) -> ConversationHistory:
        return self._history

    @property
    def config(self) -> SessionConfig:
        return self._config

    async def submit(self, user_input: str) -> TurnResult:
        """Submit user input and run the agent loop."""
        if self._state == SessionState.CLOSED:
            raise RuntimeError("Session is closed")
        if self._state == SessionState.PROCESSING:
            raise RuntimeError("Session is already processing")

        self._state = SessionState.PROCESSING
        await self._event_bus.emit(
            Event(type=EventType.SESSION_START, data={"input": user_input})
        )

        try:
            # Add user message
            self._history.add(Message.user(user_input))

            # Run agent loop
            result = await self._loop.run_turn(
                self._history,
                self._profile.system_prompt,
                self._config,
            )

            return result
        finally:
            self._state = SessionState.IDLE

    def steer(self, message: str) -> None:
        """Inject a steering message between tool rounds."""
        self._steering.steer(message)

    def follow_up(self, message: str) -> None:
        """Queue a follow-up message for after current turn."""
        self._steering.follow_up(message)

    def on_event(self, event_type: EventType, handler: Callable) -> None:
        """Register an event handler."""
        self._event_bus.on(event_type, handler)

    def on_all_events(self, handler: Callable) -> None:
        """Register a handler for all events."""
        self._event_bus.on_all(handler)

    def close(self) -> None:
        """Close the session."""
        self._state = SessionState.CLOSED
        self._loop_detector.reset()
