"""Shared test fixtures."""

import pytest


@pytest.fixture
def tmp_run_dir(tmp_path):
    """Temporary run directory for pipeline tests."""
    run_dir = tmp_path / "test_run"
    run_dir.mkdir()
    return run_dir
