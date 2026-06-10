// Package pipeline implements the DOT graph execution engine: parsing,
// validation, and the 5-phase lifecycle runner.
package pipeline

import (
	"sort"
	"strings"
	"time"
)

// NodeType enumerates the kinds of pipeline nodes.
type NodeType string

const (
	NodeStart       NodeType = "start"
	NodeExit        NodeType = "exit"
	NodeCodergen    NodeType = "codergen"
	NodeWaitHuman   NodeType = "wait.human"
	NodeConditional NodeType = "conditional"
	NodeParallel    NodeType = "parallel"
	NodeFanIn       NodeType = "fan_in"
	NodeTool        NodeType = "tool"
	NodeManagerLoop NodeType = "manager_loop"
)

// ShapeToType maps DOT node shapes onto node types.
var ShapeToType = map[string]NodeType{
	"Mdiamond":      NodeStart,
	"Msquare":       NodeExit,
	"box":           NodeCodergen,
	"hexagon":       NodeWaitHuman,
	"diamond":       NodeConditional,
	"component":     NodeParallel,
	"tripleoctagon": NodeFanIn,
	"parallelogram": NodeTool,
	"house":         NodeManagerLoop,
}

// Node is a single vertex in the pipeline graph. Attrs holds raw attribute
// strings; typed accessors coerce them on read.
type Node struct {
	ID    string            `json:"id"`
	Type  NodeType          `json:"type"`
	Label string            `json:"label"`
	Attrs map[string]string `json:"attrs"`
}

func (n *Node) attr(key string) (string, bool) {
	v, ok := n.Attrs[key]
	return v, ok
}

// Prompt returns the node's prompt attribute.
func (n *Node) Prompt() string { v, _ := n.attr("prompt"); return v }

// MaxRetries returns the node's max_retries (0 if unset).
func (n *Node) MaxRetries() int { v, ok := n.attr("max_retries"); return attrInt(v, ok, 0) }

// GoalGate reports whether the node is a goal gate.
func (n *Node) GoalGate() bool { v, ok := n.attr("goal_gate"); return attrBool(v, ok) }

// Fidelity returns the node's fidelity attribute.
func (n *Node) Fidelity() string { v, _ := n.attr("fidelity"); return v }

// Timeout returns the node's timeout as a duration, or nil if unset.
func (n *Node) Timeout() *time.Duration {
	v, ok := n.attr("timeout")
	if !ok || v == "" {
		return nil
	}
	switch pv := parseAttrValue(v).(type) {
	case int:
		d := time.Duration(pv) * time.Second
		return &d
	case float64:
		d := time.Duration(pv * float64(time.Second))
		return &d
	default:
		if secs, err := parseDuration(v); err == nil {
			d := time.Duration(secs * float64(time.Second))
			return &d
		}
		return nil
	}
}

// AutoStatus reports whether the node auto-reports success.
func (n *Node) AutoStatus() bool { v, ok := n.attr("auto_status"); return attrBool(v, ok) }

// AllowPartial reports whether a failure is downgraded to partial_success.
func (n *Node) AllowPartial() bool { v, ok := n.attr("allow_partial"); return attrBool(v, ok) }

// RetryTarget returns the node's retry_target attribute.
func (n *Node) RetryTarget() string { v, _ := n.attr("retry_target"); return v }

// LLMModel returns the node's llm_model override.
func (n *Node) LLMModel() string { v, _ := n.attr("llm_model"); return v }

// LLMProvider returns the node's llm_provider override.
func (n *Node) LLMProvider() string { v, _ := n.attr("llm_provider"); return v }

// ReasoningEffort returns the node's reasoning_effort (default "high").
func (n *Node) ReasoningEffort() string {
	if v, ok := n.attr("reasoning_effort"); ok {
		return v
	}
	return "high"
}

// ClassName returns the node's class attribute.
func (n *Node) ClassName() string { v, _ := n.attr("class"); return v }

// MaxIterations returns the per-node visit cap (0 = unlimited).
func (n *Node) MaxIterations() int { v, ok := n.attr("max_iterations"); return attrInt(v, ok, 0) }

// Skills returns the node's comma-separated skill references.
func (n *Node) Skills() []string {
	v, _ := n.attr("skills")
	var out []string
	for _, s := range strings.Split(v, ",") {
		if t := strings.TrimSpace(s); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// ThreadID returns the node's thread_id attribute.
func (n *Node) ThreadID() string { v, _ := n.attr("thread_id"); return v }

// Edge is a directed connection between two nodes.
type Edge struct {
	Source      string `json:"source"`
	Target      string `json:"target"`
	Label       string `json:"label"`
	Condition   string `json:"condition"`
	Weight      int    `json:"weight"`
	Fidelity    string `json:"fidelity"`
	ThreadID    string `json:"thread_id"`
	LoopRestart bool   `json:"loop_restart"`
}

// Graph is the parsed pipeline: nodes, edges, and graph-level attributes.
type Graph struct {
	Name                string            `json:"name"`
	Nodes               map[string]*Node  `json:"nodes"`
	Edges               []*Edge           `json:"edges"`
	Goal                string            `json:"goal"`
	ModelStylesheet     string            `json:"model_stylesheet"`
	DefaultMaxRetry     int               `json:"default_max_retry"`
	DefaultFidelity     string            `json:"default_fidelity"`
	RetryTarget         string            `json:"retry_target"`
	FallbackRetryTarget string            `json:"fallback_retry_target"`
	Attrs               map[string]string `json:"attrs"`
	// Order records node ids in declaration order, so iteration that is
	// behaviorally significant (e.g. goal-gate retry_target selection) matches
	// the Python runner's dict-insertion order rather than alphabetical order.
	Order []string `json:"-"`
}

// NewGraph returns an initialized empty graph.
func NewGraph(name string) *Graph {
	return &Graph{
		Name:            name,
		Nodes:           map[string]*Node{},
		DefaultMaxRetry: 50,
		DefaultFidelity: "compact",
		Attrs:           map[string]string{},
	}
}

// GetNode returns a node by id, or nil.
func (g *Graph) GetNode(id string) *Node { return g.Nodes[id] }

// OutgoingEdges returns edges whose source is the given node.
func (g *Graph) OutgoingEdges(id string) []*Edge {
	var out []*Edge
	for _, e := range g.Edges {
		if e.Source == id {
			out = append(out, e)
		}
	}
	return out
}

// IncomingEdges returns edges whose target is the given node.
func (g *Graph) IncomingEdges(id string) []*Edge {
	var out []*Edge
	for _, e := range g.Edges {
		if e.Target == id {
			out = append(out, e)
		}
	}
	return out
}

// StartNode returns the first start node, or nil.
func (g *Graph) StartNode() *Node {
	for _, id := range g.sortedNodeIDs() {
		if g.Nodes[id].Type == NodeStart {
			return g.Nodes[id]
		}
	}
	return nil
}

// ExitNodes returns all exit nodes.
func (g *Graph) ExitNodes() []*Node {
	var out []*Node
	for _, id := range g.sortedNodeIDs() {
		if g.Nodes[id].Type == NodeExit {
			out = append(out, g.Nodes[id])
		}
	}
	return out
}

// OrderedNodeIDs returns node ids in declaration order, appending any nodes not
// present in Order (defensive) in sorted order.
func (g *Graph) OrderedNodeIDs() []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(g.Nodes))
	for _, id := range g.Order {
		if g.Nodes[id] != nil && !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	for _, id := range g.sortedNodeIDs() {
		if !seen[id] {
			out = append(out, id)
			seen[id] = true
		}
	}
	return out
}

// sortedNodeIDs returns node ids in deterministic order.
func (g *Graph) sortedNodeIDs() []string {
	ids := make([]string, 0, len(g.Nodes))
	for id := range g.Nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
