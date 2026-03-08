"""Thread-safe pipeline context (key-value store)."""

from __future__ import annotations

import threading
from typing import Any


class PipelineContext:
    """Thread-safe key-value store shared across pipeline stages."""

    def __init__(self, initial: dict[str, Any] | None = None):
        self._data: dict[str, Any] = dict(initial or {})
        self._lock = threading.Lock()

    def get(self, key: str, default: Any = None) -> Any:
        """Get value by key. Supports dotted keys like 'step1.result'."""
        with self._lock:
            return self._get_nested(key, default)

    def set(self, key: str, value: Any) -> None:
        """Set a value. Supports dotted keys."""
        with self._lock:
            self._set_nested(key, value)

    def delete(self, key: str) -> None:
        """Delete a key."""
        with self._lock:
            parts = key.split(".")
            data = self._data
            for part in parts[:-1]:
                if isinstance(data, dict) and part in data:
                    data = data[part]
                else:
                    return
            if isinstance(data, dict):
                data.pop(parts[-1], None)

    def update(self, mapping: dict[str, Any]) -> None:
        """Merge a dict into the context."""
        with self._lock:
            for key, value in mapping.items():
                self._set_nested(key, value)

    def snapshot(self) -> dict[str, Any]:
        """Return a serializable copy of all data."""
        import copy
        with self._lock:
            return copy.deepcopy(self._data)

    def restore(self, snapshot: dict[str, Any]) -> None:
        """Restore from a snapshot."""
        import copy
        with self._lock:
            self._data = copy.deepcopy(snapshot)

    def _get_nested(self, key: str, default: Any = None) -> Any:
        parts = key.split(".")
        data = self._data
        for part in parts:
            if isinstance(data, dict) and part in data:
                data = data[part]
            else:
                return default
        return data

    def _set_nested(self, key: str, value: Any) -> None:
        parts = key.split(".")
        data = self._data
        for part in parts[:-1]:
            if part not in data or not isinstance(data.get(part), dict):
                data[part] = {}
            data = data[part]
        data[parts[-1]] = value

    def __contains__(self, key: str) -> bool:
        return self.get(key) is not None

    def __repr__(self) -> str:
        return f"PipelineContext({self._data})"
