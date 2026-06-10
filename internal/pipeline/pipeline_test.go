package pipeline

import (
	"context"
	"path/filepath"
	"testing"
)

func TestConditions(t *testing.T) {
	ctx := NewPipelineContext(nil)
	ctx.Set("flag", "yes")

	cases := []struct {
		expr           string
		outcome, label string
		want           bool
	}{
		{"", "", "", true},
		{"outcome=success", "success", "", true},
		{"outcome=success", "fail", "", false},
		{"outcome!=fail", "success", "", true},
		{"context.flag=yes", "", "", true},
		{"context.flag=no", "", "", false},
		{"outcome=success && context.flag=yes", "success", "", true},
		{"outcome=success && context.flag=no", "success", "", false},
		{"preferred_label=retry", "", "retry", true},
	}
	for _, c := range cases {
		got, err := EvaluateCondition(c.expr, ctx, c.outcome, c.label)
		if err != nil {
			t.Errorf("EvaluateCondition(%q): %v", c.expr, err)
			continue
		}
		if got != c.want {
			t.Errorf("EvaluateCondition(%q, outcome=%q, label=%q) = %v, want %v", c.expr, c.outcome, c.label, got, c.want)
		}
	}
}

func TestGoalGate(t *testing.T) {
	g := NewGraph("t")
	g.Nodes["a"] = &Node{ID: "a", Type: NodeCodergen, Attrs: map[string]string{"goal_gate": "true", "retry_target": "a"}}
	g.Nodes["b"] = &Node{ID: "b", Type: NodeCodergen, Attrs: map[string]string{}}

	res := CheckGoalGates(g, map[string]string{"a": "fail", "b": "success"})
	if res.Satisfied {
		t.Error("expected unsatisfied gate")
	}
	if res.RetryTarget != "a" {
		t.Errorf("retry target = %q, want a", res.RetryTarget)
	}

	res = CheckGoalGates(g, map[string]string{"a": "success"})
	if !res.Satisfied {
		t.Error("expected satisfied gate")
	}
}

func TestCheckpointRoundtrip(t *testing.T) {
	dir := t.TempDir()
	cp := Checkpoint{
		RunID:           "abc",
		CurrentNode:     "n1",
		CompletedNodes:  []string{"start", "n1"},
		NodeRetries:     map[string]int{"n1": 2},
		ContextSnapshot: map[string]any{"graph": map[string]any{"goal": "x"}},
	}
	if err := SaveCheckpoint(cp, dir); err != nil {
		t.Fatalf("save: %v", err)
	}
	loaded, err := LoadCheckpoint(dir)
	if err != nil || loaded == nil {
		t.Fatalf("load: %v", err)
	}
	if loaded.RunID != "abc" || loaded.CurrentNode != "n1" || loaded.NodeRetries["n1"] != 2 {
		t.Errorf("roundtrip mismatch: %+v", loaded)
	}
}

func TestContextDottedKeys(t *testing.T) {
	c := NewPipelineContext(nil)
	c.Set("a.b.c", "deep")
	if got := c.GetString("a.b.c"); got != "deep" {
		t.Errorf("nested get = %q", got)
	}
	snap := c.Snapshot()
	c2 := NewPipelineContext(nil)
	c2.Restore(snap)
	if got := c2.GetString("a.b.c"); got != "deep" {
		t.Errorf("restored nested get = %q", got)
	}
}

// TestRunnerNoCodergen exercises the full 5-phase runner offline with a graph
// that needs no LLM client (start -> tool(echo) -> exit).
func TestRunnerNoCodergen(t *testing.T) {
	dot := `digraph t {
		start [shape=Mdiamond];
		echo  [shape=parallelogram, command="echo hi"];
		done  [shape=Msquare];
		start -> echo -> done;
	}`
	runner := NewPipelineRunner(RunnerOptions{})
	dir := t.TempDir()
	result := runner.Run(context.Background(), dot, RunParams{RunDir: dir})
	if !result.Success {
		t.Fatalf("run failed: %v", result.Errors)
	}
	wantOrder := []string{"start", "echo", "done"}
	if len(result.NodesExecuted) != 3 {
		t.Fatalf("nodes executed = %v, want %v", result.NodesExecuted, wantOrder)
	}
	for i, n := range wantOrder {
		if result.NodesExecuted[i] != n {
			t.Errorf("node[%d] = %q, want %q", i, result.NodesExecuted[i], n)
		}
	}
	// Checkpoint + status artifacts should exist.
	if cp, _ := LoadCheckpoint(dir); cp == nil {
		t.Error("checkpoint not written")
	}
	if st, _ := ReadStatus("echo", dir); st == nil || st.Outcome != "success" {
		t.Errorf("echo status = %v", st)
	}
	_ = filepath.Join(dir, "echo", "response.md")
}
