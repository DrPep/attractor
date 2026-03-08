"""Unified LLM Client - provider-agnostic interface for multiple LLM providers."""

from .models import (
    ContentKind,
    ContentPart,
    FinishReason,
    ImageData,
    Message,
    Request,
    Response,
    ResponseFormat,
    Role,
    ToolCallData,
    ToolChoice,
    ToolDefinition,
    ToolResultData,
    Usage,
    Warning,
)
from .adapter import ProviderAdapter
from .client import Client, generate, stream, set_default_client, get_default_client
from .catalog import ModelInfo, get_model_info, list_models, resolve_provider
from .streaming import StreamAccumulator, StreamEvent, StreamEventType
from .middleware import (
    Middleware,
    LoggingMiddleware,
    RetryMiddleware,
    UsageTrackingMiddleware,
)

__all__ = [
    "Client",
    "ContentKind",
    "ContentPart",
    "FinishReason",
    "ImageData",
    "Message",
    "Middleware",
    "ModelInfo",
    "ProviderAdapter",
    "Request",
    "Response",
    "ResponseFormat",
    "Role",
    "StreamAccumulator",
    "StreamEvent",
    "StreamEventType",
    "ToolCallData",
    "ToolChoice",
    "ToolDefinition",
    "ToolResultData",
    "Usage",
    "Warning",
    "generate",
    "get_default_client",
    "get_model_info",
    "list_models",
    "resolve_provider",
    "set_default_client",
    "stream",
    "LoggingMiddleware",
    "RetryMiddleware",
    "UsageTrackingMiddleware",
]
