"""Tests for loop detection."""

from attractor.agent.loop_detector import LoopDetector


def test_no_loop():
    detector = LoopDetector(threshold=3)
    result = detector.record("read_file", {"path": "a.py"})
    assert not result.is_looping
    result = detector.record("write_file", {"path": "b.py"})
    assert not result.is_looping


def test_detects_simple_loop():
    detector = LoopDetector(threshold=3)
    for i in range(2):
        result = detector.record("read_file", {"path": "same.py"})
    assert not result.is_looping

    result = detector.record("read_file", {"path": "same.py"})
    assert result.is_looping
    assert "repeated" in result.description.lower()


def test_different_args_no_loop():
    detector = LoopDetector(threshold=3)
    for i in range(5):
        result = detector.record("read_file", {"path": f"file_{i}.py"})
    assert not result.is_looping


def test_reset():
    detector = LoopDetector(threshold=3)
    for i in range(3):
        detector.record("read_file", {"path": "same.py"})
    detector.reset()
    result = detector.record("read_file", {"path": "same.py"})
    assert not result.is_looping
