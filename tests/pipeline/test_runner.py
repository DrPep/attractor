"""Tests for pipeline runner."""

import pytest
from pathlib import Path

from attractor.pipeline.runner import PipelineRunner, RunResult
from attractor.pipeline.interviewer import QueueInterviewer, Answer


@pytest.mark.asyncio
async def test_simple_pipeline(tmp_run_dir):
    dot = '''
    digraph test {
        start [shape=Mdiamond];
        finish [shape=Msquare];
        start -> finish;
    }
    '''
    runner = PipelineRunner()
    result = await runner.run(dot, run_dir=tmp_run_dir)
    assert result.success
    assert "start" in result.nodes_executed
    assert "finish" in result.nodes_executed


@pytest.mark.asyncio
async def test_linear_pipeline(tmp_run_dir):
    dot = '''
    digraph test {
        start [shape=Mdiamond];
        step1 [shape=parallelogram, command="echo hello"];
        step2 [shape=parallelogram, command="echo world"];
        finish [shape=Msquare];
        start -> step1 -> step2 -> finish;
    }
    '''
    runner = PipelineRunner()
    result = await runner.run(dot, run_dir=tmp_run_dir)
    assert result.success
    assert len(result.nodes_executed) == 4


@pytest.mark.asyncio
async def test_conditional_pipeline(tmp_run_dir):
    dot = '''
    digraph test {
        start [shape=Mdiamond];
        check [shape=diamond];
        pass_node [shape=Msquare, label="pass"];
        fail_node [shape=Msquare, label="fail"];
        start -> check;
        check -> pass_node [condition="outcome = success", weight=10];
        check -> fail_node [condition="outcome = fail"];
    }
    '''
    runner = PipelineRunner()
    result = await runner.run(dot, run_dir=tmp_run_dir)
    assert result.success
    assert "pass_node" in result.nodes_executed


@pytest.mark.asyncio
async def test_wait_human_pipeline(tmp_run_dir):
    dot = '''
    digraph test {
        start [shape=Mdiamond];
        review [shape=hexagon, label="Approve?"];
        done [shape=Msquare];
        start -> review;
        review -> done [label="Approve"];
    }
    '''
    interviewer = QueueInterviewer(["Approve"])
    runner = PipelineRunner(interviewer=interviewer)
    result = await runner.run(dot, run_dir=tmp_run_dir)
    assert result.success


@pytest.mark.asyncio
async def test_goal_gate_satisfied(tmp_run_dir):
    dot = '''
    digraph test {
        start [shape=Mdiamond];
        critical [shape=parallelogram, command="echo ok", goal_gate=true];
        finish [shape=Msquare];
        start -> critical -> finish;
    }
    '''
    runner = PipelineRunner()
    result = await runner.run(dot, run_dir=tmp_run_dir)
    assert result.success


@pytest.mark.asyncio
async def test_checkpoint_created(tmp_run_dir):
    dot = '''
    digraph test {
        start [shape=Mdiamond];
        finish [shape=Msquare];
        start -> finish;
    }
    '''
    runner = PipelineRunner()
    await runner.run(dot, run_dir=tmp_run_dir)
    assert (tmp_run_dir / "checkpoint.json").exists()


@pytest.mark.asyncio
async def test_status_files_created(tmp_run_dir):
    dot = '''
    digraph test {
        start [shape=Mdiamond];
        step [shape=parallelogram, command="echo test"];
        finish [shape=Msquare];
        start -> step -> finish;
    }
    '''
    runner = PipelineRunner()
    await runner.run(dot, run_dir=tmp_run_dir)
    assert (tmp_run_dir / "start" / "status.json").exists()
    assert (tmp_run_dir / "step" / "status.json").exists()
    assert (tmp_run_dir / "finish" / "status.json").exists()


@pytest.mark.asyncio
async def test_node_callbacks(tmp_run_dir):
    started = []
    ended = []

    dot = '''
    digraph test {
        start [shape=Mdiamond];
        finish [shape=Msquare];
        start -> finish;
    }
    '''
    runner = PipelineRunner(
        on_node_start=lambda n: started.append(n),
        on_node_end=lambda n, s: ended.append((n, s)),
    )
    await runner.run(dot, run_dir=tmp_run_dir)
    assert "start" in started
    assert "finish" in started
    assert len(ended) == 2
