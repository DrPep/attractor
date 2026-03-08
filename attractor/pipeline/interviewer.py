"""Human-in-the-loop interface."""

from __future__ import annotations

import asyncio
from abc import ABC, abstractmethod
from enum import Enum
from typing import Any, Awaitable, Callable

from pydantic import BaseModel, Field


class QuestionType(str, Enum):
    SINGLE_SELECT = "single_select"
    MULTI_SELECT = "multi_select"
    FREE_TEXT = "free_text"
    CONFIRM = "confirm"


class Question(BaseModel):
    text: str
    type: QuestionType = QuestionType.SINGLE_SELECT
    choices: list[str] = Field(default_factory=list)
    default_choice: str = ""
    timeout: float | None = None


class Answer(BaseModel):
    value: str | list[str] = ""


class Interviewer(ABC):
    """Protocol for human-in-the-loop interaction."""

    @abstractmethod
    async def ask(self, question: Question) -> Answer: ...


class AutoApproveInterviewer(Interviewer):
    """Always selects the first option. For automation/testing."""

    async def ask(self, question: Question) -> Answer:
        if question.type == QuestionType.CONFIRM:
            return Answer(value="yes")
        if question.choices:
            return Answer(value=question.choices[0])
        if question.default_choice:
            return Answer(value=question.default_choice)
        return Answer(value="")


class ConsoleInterviewer(Interviewer):
    """Terminal-based interactive prompts."""

    async def ask(self, question: Question) -> Answer:
        print(f"\n{question.text}")

        if question.type == QuestionType.CONFIRM:
            default = question.default_choice or "y"
            response = await asyncio.to_thread(
                input, f"[y/n] (default: {default}): "
            )
            value = response.strip().lower() or default
            return Answer(value="yes" if value.startswith("y") else "no")

        if question.type in (QuestionType.SINGLE_SELECT, QuestionType.MULTI_SELECT):
            for i, choice in enumerate(question.choices, 1):
                print(f"  {i}. {choice}")
            default_hint = f" (default: {question.default_choice})" if question.default_choice else ""
            response = await asyncio.to_thread(
                input, f"Choose{default_hint}: "
            )
            response = response.strip() or question.default_choice

            if question.type == QuestionType.MULTI_SELECT:
                indices = [int(x.strip()) for x in response.split(",") if x.strip().isdigit()]
                values = [question.choices[i - 1] for i in indices if 0 < i <= len(question.choices)]
                return Answer(value=values)
            else:
                # Try as index
                if response.isdigit():
                    idx = int(response)
                    if 0 < idx <= len(question.choices):
                        return Answer(value=question.choices[idx - 1])
                # Try as label match
                for choice in question.choices:
                    if choice.lower().startswith(response.lower()):
                        return Answer(value=choice)
                return Answer(value=response)

        # FREE_TEXT
        response = await asyncio.to_thread(input, "> ")
        return Answer(value=response.strip())


class CallbackInterviewer(Interviewer):
    """Delegates to a provided async function."""

    def __init__(self, callback: Callable[[Question], Awaitable[Answer]]):
        self._callback = callback

    async def ask(self, question: Question) -> Answer:
        return await self._callback(question)


class QueueInterviewer(Interviewer):
    """Pre-filled answer queue for deterministic testing."""

    def __init__(self, answers: list[Answer | str] | None = None):
        self._queue: list[Answer] = []
        for a in (answers or []):
            if isinstance(a, str):
                self._queue.append(Answer(value=a))
            else:
                self._queue.append(a)

    async def ask(self, question: Question) -> Answer:
        if self._queue:
            return self._queue.pop(0)
        if question.default_choice:
            return Answer(value=question.default_choice)
        if question.choices:
            return Answer(value=question.choices[0])
        return Answer(value="")


class RecordingInterviewer(Interviewer):
    """Wraps another interviewer, recording Q&A pairs."""

    def __init__(self, delegate: Interviewer):
        self._delegate = delegate
        self.recordings: list[tuple[Question, Answer]] = []

    async def ask(self, question: Question) -> Answer:
        answer = await self._delegate.ask(question)
        self.recordings.append((question, answer))
        return answer
