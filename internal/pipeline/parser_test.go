package pipeline

import "testing"

func TestParseHello(t *testing.T) {
	g, err := ParseDOTFile("../../examples/hello.dot")
	if err != nil {
		t.Fatalf("parse hello.dot: %v", err)
	}
	if g.Goal != "Say hello to the user" {
		t.Errorf("goal = %q", g.Goal)
	}
	if len(g.Nodes) != 3 {
		t.Errorf("nodes = %d, want 3 (%v)", len(g.Nodes), g.sortedNodeIDs())
	}
	if len(g.Edges) != 2 {
		t.Errorf("edges = %d, want 2", len(g.Edges))
	}
	if s := g.StartNode(); s == nil || s.ID != "start" {
		t.Errorf("start node = %v", s)
	}
	greet := g.GetNode("greet")
	if greet == nil || greet.Type != NodeCodergen {
		t.Fatalf("greet node missing or wrong type: %v", greet)
	}
	if greet.Prompt() != "Say hello in a creative way" {
		t.Errorf("greet prompt = %q", greet.Prompt())
	}
}

func TestParseCLI(t *testing.T) {
	g, err := ParseDOTFile("../../examples/cli.dot")
	if err != nil {
		t.Fatalf("parse cli.dot: %v", err)
	}
	if len(g.Nodes) != 5 {
		t.Errorf("nodes = %d, want 5 (%v)", len(g.Nodes), g.sortedNodeIDs())
	}
	if len(g.Edges) != 5 {
		t.Errorf("edges = %d, want 5", len(g.Edges))
	}
	code := g.GetNode("code")
	if code == nil {
		t.Fatal("code node missing")
	}
	if code.MaxIterations() != 5 {
		t.Errorf("code max_iterations = %d, want 5", code.MaxIterations())
	}
	if code.Prompt() != "Implement $goal" {
		t.Errorf("code prompt = %q", code.Prompt())
	}
	// Conditional edges
	var retryEdge *Edge
	for _, e := range g.Edges {
		if e.Source == "check" && e.Target == "code" {
			retryEdge = e
		}
	}
	if retryEdge == nil {
		t.Fatal("check->code edge missing")
	}
	if retryEdge.Label != "retry" || retryEdge.Condition != "outcome=fail" {
		t.Errorf("retry edge = %+v", retryEdge)
	}
}

func TestParseRejectsUndirected(t *testing.T) {
	_, err := ParseDOT("graph g { a -- b }")
	if err == nil {
		t.Fatal("expected error for undirected graph")
	}
}

func TestParseRejectsStrict(t *testing.T) {
	_, err := ParseDOT("strict digraph g { a -> b }")
	if err == nil {
		t.Fatal("expected error for strict modifier")
	}
}

func TestValidateHello(t *testing.T) {
	g, err := ParseDOTFile("../../examples/hello.dot")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := ValidateOrRaise(g, nil); err != nil {
		t.Errorf("hello.dot should validate: %v", err)
	}
}

func TestEdgeSelection(t *testing.T) {
	g, err := ParseDOTFile("../../examples/cli.dot")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ctx := NewPipelineContext(nil)
	edges := g.OutgoingEdges("check")
	// outcome=fail should route to code via the retry edge
	e := SelectEdge(edges, "fail", "", nil, ctx)
	if e == nil || e.Target != "code" {
		t.Errorf("fail outcome routed to %v, want code", e)
	}
	e = SelectEdge(edges, "success", "", nil, ctx)
	if e == nil || e.Target != "done" {
		t.Errorf("success outcome routed to %v, want done", e)
	}
}
