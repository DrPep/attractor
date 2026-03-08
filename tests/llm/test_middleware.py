"""Tests for middleware."""

import pytest

from attractor.llm.middleware import (
    LoggingMiddleware, RetryMiddleware, UsageTrackingMiddleware,
)
from attractor.llm.models import (
    FinishReason, Message, Request, Response, Usage,
)


def make_response(**kwargs):
    defaults = dict(
        id="r1", model="test", provider="test",
        message=Message.assistant("ok"),
        finish_reason=FinishReason.STOP,
        usage=Usage(input_tokens=10, output_tokens=5),
    )
    defaults.update(kwargs)
    return Response(**defaults)


@pytest.mark.asyncio
async def test_logging_middleware(caplog):
    import logging
    mw = LoggingMiddleware()
    req = Request(model="test", messages=[Message.user("hi")])

    async def next_fn(r):
        return make_response()

    with caplog.at_level(logging.INFO):
        resp = await mw(req, next_fn)
    assert resp.text == "ok"


@pytest.mark.asyncio
async def test_usage_tracking_middleware():
    mw = UsageTrackingMiddleware()
    req = Request(model="test", messages=[Message.user("hi")])

    async def next_fn(r):
        return make_response()

    await mw(req, next_fn)
    await mw(req, next_fn)

    assert mw.call_count == 2
    assert mw.total_usage.input_tokens == 20
    assert mw.total_usage.output_tokens == 10
