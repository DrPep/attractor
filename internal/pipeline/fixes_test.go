package pipeline

import (
	"context"
	"strings"
	"testing"
	"time"
)

// stubHandler returns a fixed outcome after an optional delay (cancellable).
type stubHandler struct {
	status string
	delay  time.Duration
}

func (s *stubHandler) Execute(ctx context.Context, node *Node, pctx *PipelineContext, g *Graph, runDir string) (Outcome, error) {
	if s.delay > 0 {
		select {
		case <-time.After(s.delay):
		case <-ctx.Done():
			return Outcome{Status: "fail", Notes: "cancelled"}, nil
		}
	}
	return Outcome{Status: s.status}, nil
}

// Fix 1: first_success returns on the first successful branch without waiting
// for (and after cancelling) the slow branch.
func TestParallelFirstSuccessShortCircuits(t *testing.T) {
	reg := NewHandlerRegistry()
	reg.Register(NodeConditional, &stubHandler{status: "success"})
	reg.Register(NodeTool, &stubHandler{status: "fail", delay: 3 * time.Second})
	ph := &ParallelHandler{registry: reg}

	g := NewGraph("t")
	g.Nodes["p"] = &Node{ID: "p", Type: NodeParallel, Attrs: map[string]string{"join_policy": "first_success"}}
	g.Nodes["fast"] = &Node{ID: "fast", Type: NodeConditional, Attrs: map[string]string{}}
	g.Nodes["slow"] = &Node{ID: "slow", Type: NodeTool, Attrs: map[string]string{}}
	g.Edges = []*Edge{{Source: "p", Target: "fast"}, {Source: "p", Target: "slow"}}

	start := time.Now()
	oc, _ := ph.Execute(context.Background(), g.Nodes["p"], NewPipelineContext(nil), g, t.TempDir())
	elapsed := time.Since(start)

	if oc.Status != "success" {
		t.Fatalf("status = %q, want success", oc.Status)
	}
	if elapsed > 2*time.Second {
		t.Errorf("did not short-circuit; took %v (slow branch is 3s)", elapsed)
	}
}

func startExitStubRegistry(toolStatus string) *HandlerRegistry {
	reg := NewHandlerRegistry()
	reg.Register(NodeStart, &StartHandler{})
	reg.Register(NodeExit, &ExitHandler{})
	reg.Register(NodeTool, &stubHandler{status: toolStatus})
	return reg
}

func nestedRetryCount(ctx map[string]any, node string) (int, bool) {
	internal, ok := ctx["internal"].(map[string]any)
	if !ok {
		return 0, false
	}
	rc, ok := internal["retry_count"].(map[string]any)
	if !ok {
		return 0, false
	}
	v, ok := rc[node]
	if !ok {
		return 0, false
	}
	return toInt(v), true
}

// Fix 2: internal.retry_count is set for a "retry" outcome but not for "fail".
func TestRetryCountOnlyOnRetry(t *testing.T) {
	dot := `digraph t {
		start [shape=Mdiamond];
		t [shape=parallelogram, max_retries=1];
		done [shape=Msquare];
		start -> t -> done;
	}`

	failRunner := NewPipelineRunner(RunnerOptions{HandlerRegistry: startExitStubRegistry("fail")})
	failRes := failRunner.Run(context.Background(), dot, RunParams{RunDir: t.TempDir()})
	if _, present := nestedRetryCount(failRes.FinalContext, "t"); present {
		t.Errorf("retry_count should be absent for a fail outcome")
	}

	retryRunner := NewPipelineRunner(RunnerOptions{HandlerRegistry: startExitStubRegistry("retry")})
	retryRes := retryRunner.Run(context.Background(), dot, RunParams{RunDir: t.TempDir()})
	if n, present := nestedRetryCount(retryRes.FinalContext, "t"); !present || n != 1 {
		t.Errorf("retry_count = (%d, present=%v), want (1, true) for a retry outcome", n, present)
	}
}

// Fix 3: goal-gate retry_target is chosen by declaration order, not alphabetical.
func TestGoalGateDeclarationOrder(t *testing.T) {
	g := NewGraph("t")
	g.Nodes["zzz"] = &Node{ID: "zzz", Type: NodeCodergen, Attrs: map[string]string{"goal_gate": "true", "retry_target": "code"}}
	g.Nodes["aaa"] = &Node{ID: "aaa", Type: NodeCodergen, Attrs: map[string]string{"goal_gate": "true", "retry_target": "fix"}}
	g.Order = []string{"zzz", "aaa"} // zzz declared first

	res := CheckGoalGates(g, map[string]string{"zzz": "fail", "aaa": "fail"})
	if res.Satisfied {
		t.Fatal("expected unsatisfied")
	}
	if res.RetryTarget != "code" {
		t.Errorf("retry target = %q, want code (first declared)", res.RetryTarget)
	}
}

// Fix 4: attrBool matches Python bool() truthiness for non-numeric strings.
func TestAttrBoolTruthiness(t *testing.T) {
	cases := map[string]bool{"yes": true, "on": true, "true": true, "1": true, "false": false, "0": false, "": false}
	for raw, want := range cases {
		n := &Node{Attrs: map[string]string{"goal_gate": raw}}
		if got := n.GoalGate(); got != want {
			t.Errorf("goal_gate=%q -> %v, want %v", raw, got, want)
		}
	}
	absent := &Node{Attrs: map[string]string{}}
	if absent.GoalGate() {
		t.Error("absent goal_gate should be false")
	}
}

// Fix 5: exhausting the runner via the goal-gate continue path still records the
// iteration-limit error.
func TestMaxIterationLimitRecorded(t *testing.T) {
	// c -> c is always chosen over c -> done (lexical tiebreak), so the runner
	// loops on c until the safety cap.
	dot := `digraph t {
		start [shape=Mdiamond];
		c [shape=diamond];
		done [shape=Msquare];
		start -> c;
		c -> c;
		c -> done;
	}`
	runner := NewPipelineRunner(RunnerOptions{})
	res := runner.Run(context.Background(), dot, RunParams{RunDir: t.TempDir()})

	found := false
	for _, e := range res.Errors {
		if strings.Contains(e, "Maximum iteration limit") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'Maximum iteration limit reached', got errors: %v", res.Errors)
	}
}
