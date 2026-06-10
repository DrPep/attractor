package pipeline

import (
	"fmt"
	"strings"

	"github.com/nigelpepper/attractor/internal/aerr"
)

// Severity classifies a diagnostic.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
	SeverityInfo    Severity = "info"
)

// Diagnostic is a single validation finding.
type Diagnostic struct {
	Severity     Severity
	Message      string
	NodeID       string
	SuggestedFix string
}

// Validate checks a pipeline graph and returns diagnostics. knownSkills, when
// non-nil, enables warnings about unknown skill references.
func Validate(g *Graph, knownSkills map[string]bool) []Diagnostic {
	var diags []Diagnostic
	add := func(sev Severity, msg, nodeID, fix string) {
		diags = append(diags, Diagnostic{Severity: sev, Message: msg, NodeID: nodeID, SuggestedFix: fix})
	}

	var startNodes, exitNodes []*Node
	for _, id := range g.sortedNodeIDs() {
		switch g.Nodes[id].Type {
		case NodeStart:
			startNodes = append(startNodes, g.Nodes[id])
		case NodeExit:
			exitNodes = append(exitNodes, g.Nodes[id])
		}
	}

	switch {
	case len(startNodes) == 0:
		add(SeverityError, "No start node found (use shape=Mdiamond)", "", "Add a node with shape=Mdiamond")
	case len(startNodes) > 1:
		ids := make([]string, len(startNodes))
		for i, n := range startNodes {
			ids[i] = n.ID
		}
		add(SeverityError, fmt.Sprintf("Multiple start nodes found: %v", ids), "", "Keep only one start node")
	}

	if len(exitNodes) == 0 {
		add(SeverityError, "No exit node found (use shape=Msquare)", "", "Add a node with shape=Msquare")
	}

	for _, start := range startNodes {
		if len(g.IncomingEdges(start.ID)) > 0 {
			add(SeverityError, fmt.Sprintf("Start node '%s' has incoming edges", start.ID), start.ID,
				"Remove edges pointing to the start node")
		}
	}

	for _, exit := range exitNodes {
		if len(g.OutgoingEdges(exit.ID)) > 0 {
			add(SeverityError, fmt.Sprintf("Exit node '%s' has outgoing edges", exit.ID), exit.ID,
				"Remove edges from the exit node")
		}
	}

	if len(startNodes) > 0 {
		reachable := findReachable(g, startNodes[0].ID)
		for _, id := range g.sortedNodeIDs() {
			if !reachable[id] {
				add(SeverityWarning, fmt.Sprintf("Node '%s' is unreachable from start", id), id,
					"Add an edge path from start to this node, or remove it")
			}
		}
	}

	for _, e := range g.Edges {
		if g.Nodes[e.Source] == nil {
			add(SeverityError, fmt.Sprintf("Edge source '%s' does not exist", e.Source), "", "")
		}
		if g.Nodes[e.Target] == nil {
			add(SeverityError, fmt.Sprintf("Edge target '%s' does not exist", e.Target), "", "")
		}
	}

	for _, e := range g.Edges {
		if e.Condition != "" {
			for _, msg := range ValidateCondition(e.Condition) {
				add(SeverityError, fmt.Sprintf("Condition syntax error on edge %s->%s: %s", e.Source, e.Target, msg), "", "")
			}
		}
	}

	for _, id := range g.sortedNodeIDs() {
		node := g.Nodes[id]
		if node.Type == NodeCodergen && node.Prompt() == "" {
			add(SeverityWarning, fmt.Sprintf("Codergen node '%s' has no prompt", node.ID), node.ID, "Add a prompt attribute")
		}
	}

	if knownSkills != nil {
		for _, id := range g.sortedNodeIDs() {
			node := g.Nodes[id]
			for _, skill := range node.Skills() {
				if !knownSkills[skill] {
					add(SeverityWarning, fmt.Sprintf("Node '%s' references unknown skill '%s'", node.ID, skill), node.ID,
						fmt.Sprintf("Register skill '%s' or check for typos", skill))
				}
			}
		}
	}

	graphFallback := g.RetryTarget
	if graphFallback == "" {
		graphFallback = g.FallbackRetryTarget
	}
	for _, id := range g.sortedNodeIDs() {
		node := g.Nodes[id]
		if node.GoalGate() && node.RetryTarget() == "" && graphFallback == "" {
			add(SeverityWarning, fmt.Sprintf("Goal gate node '%s' has no retry_target", node.ID), node.ID,
				"Add retry_target on the node, or graph-level retry_target/fallback_retry_target")
		}
	}

	return diags
}

// ValidateOrRaise validates and returns an error if any ERROR diagnostics
// exist; otherwise it returns the non-error diagnostics (warnings/info).
func ValidateOrRaise(g *Graph, knownSkills map[string]bool) ([]Diagnostic, error) {
	diags := Validate(g, knownSkills)
	var errs, rest []string
	var nonErr []Diagnostic
	for _, d := range diags {
		if d.Severity == SeverityError {
			errs = append(errs, d.Message)
		} else {
			nonErr = append(nonErr, d)
		}
	}
	_ = rest
	if len(errs) > 0 {
		return nil, &aerr.ValidationError{Msg: "Graph validation failed: " + strings.Join(errs, "; ")}
	}
	return nonErr, nil
}

func findReachable(g *Graph, startID string) map[string]bool {
	visited := map[string]bool{}
	queue := []string{startID}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur] {
			continue
		}
		visited[cur] = true
		for _, e := range g.OutgoingEdges(cur) {
			if !visited[e.Target] {
				queue = append(queue, e.Target)
			}
		}
	}
	return visited
}
