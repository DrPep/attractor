package pipeline

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/awalterschulze/gographviz"

	"github.com/nigelpepper/attractor/internal/aerr"
)

var validNodeID = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

var (
	reBlockComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	reLineComment  = regexp.MustCompile(`//[^\n]*`)
	reStrict       = regexp.MustCompile(`(?i)\bstrict\b`)
	reDigraph      = regexp.MustCompile(`(?i)\bdigraph\b`)
	reGraphAttrKV  = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*\s*=\s*"(?:[^"\\]|\\.)*"`)
	reAttrBlock    = regexp.MustCompile(`\[[^\]]*\]`)
	reQuoted       = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)
	reKV           = regexp.MustCompile(`([A-Za-z_][A-Za-z0-9_]*)\s*=\s*(?:"(?:[^"\\]|\\.)*"|[^,\]\s]+)`)
)

// ParseDOTFile reads a DOT file from disk and parses it into a Graph.
func ParseDOTFile(path string) (*Graph, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, &aerr.ParseError{Msg: fmt.Sprintf("Failed to read DOT file: %v", err)}
	}
	return ParseDOT(string(data))
}

// ParseDOT parses a DOT source string into a Graph, enforcing the constrained
// spec via preValidate before handing off to the gographviz parser.
//
// We parse to an AST and analyse it through a permissive collector rather than
// gographviz.Read, because Read rejects non-standard attribute names (goal,
// prompt, condition, ...) that this DSL relies on.
func ParseDOT(source string) (*Graph, error) {
	if err := preValidate(source); err != nil {
		return nil, err
	}

	astGraph, err := gographviz.Parse([]byte(source))
	if err != nil {
		return nil, &aerr.ParseError{Msg: fmt.Sprintf("Failed to parse DOT: %v", err)}
	}

	col := newCollector()
	if err := gographviz.Analyse(astGraph, col); err != nil {
		return nil, &aerr.ParseError{Msg: fmt.Sprintf("Failed to parse DOT: %v", err)}
	}
	if !col.directed {
		return nil, &aerr.ParseError{Msg: "Only digraph is supported"}
	}

	g := NewGraph(stripQuotes(col.name))

	// Graph-level attributes.
	for k, v := range col.graphAttrs {
		g.Attrs[k] = stripQuotes(v)
	}
	g.Goal = g.Attrs["goal"]
	g.ModelStylesheet = g.Attrs["model_stylesheet"]
	g.DefaultFidelity = orDefault(g.Attrs["default_fidelity"], "compact")
	g.RetryTarget = g.Attrs["retry_target"]
	g.FallbackRetryTarget = g.Attrs["fallback_retry_target"]
	g.DefaultMaxRetry = atoiDefault(g.Attrs["default_max_retry"], 50)

	// Nodes.
	for _, dn := range col.nodes {
		name := stripQuotes(dn.name)
		if name == "node" || name == "edge" || name == "graph" || name == "" {
			continue
		}
		g.Nodes[name] = parseNode(name, dn.attrs)
		g.Order = append(g.Order, name)
	}

	// Edges.
	for _, de := range col.edges {
		g.Edges = append(g.Edges, parseEdge(de))
	}

	// Validate all node ids (including edge endpoints) are bare identifiers.
	allIDs := map[string]bool{}
	for id := range g.Nodes {
		allIDs[id] = true
	}
	for _, e := range g.Edges {
		allIDs[e.Source] = true
		allIDs[e.Target] = true
	}
	for id := range allIDs {
		if !validNodeID.MatchString(id) {
			return nil, &aerr.ParseError{Msg: fmt.Sprintf(
				"Node ID '%s' is not a valid bare identifier. IDs must match [A-Za-z_][A-Za-z0-9_]*. "+
					"Use the 'label' attribute for human-readable names.", id)}
		}
	}

	// Ensure every edge endpoint has a node entry (default to codergen).
	for _, e := range g.Edges {
		for _, id := range []string{e.Source, e.Target} {
			if g.Nodes[id] == nil {
				g.Nodes[id] = &Node{ID: id, Type: NodeCodergen, Label: id, Attrs: map[string]string{}}
				g.Order = append(g.Order, id)
			}
		}
	}

	return g, nil
}

func parseNode(id string, attrs map[string]string) *Node {
	m := map[string]string{}
	for k, v := range attrs {
		m[k] = stripQuotes(v)
	}

	label := id
	if l, ok := m["label"]; ok {
		label = l
		delete(m, "label")
	}
	shape := "box"
	if s, ok := m["shape"]; ok {
		shape = s
		delete(m, "shape")
	}

	var nodeType NodeType
	if t, ok := m["type"]; ok {
		delete(m, "type")
		if nt, valid := knownNodeType(t); valid {
			nodeType = nt
		} else {
			nodeType = shapeType(shape)
		}
	} else {
		nodeType = shapeType(shape)
	}

	return &Node{ID: id, Type: nodeType, Label: label, Attrs: m}
}

func parseEdge(de collectedEdge) *Edge {
	get := func(key string) string { return stripQuotes(de.attrs[key]) }
	weight := 0
	if w := get("weight"); w != "" {
		if i, err := strconv.Atoi(w); err == nil {
			weight = i
		}
	}
	loopRestart := false
	if lr := get("loop_restart"); lr != "" {
		loopRestart = attrBool(lr, true)
	}
	return &Edge{
		Source:      stripQuotes(de.src),
		Target:      stripQuotes(de.dst),
		Label:       get("label"),
		Condition:   get("condition"),
		Weight:      weight,
		Fidelity:    get("fidelity"),
		ThreadID:    get("thread_id"),
		LoopRestart: loopRestart,
	}
}

// --- permissive AST collector implementing gographviz.Interface ---

type collectedNode struct {
	name  string
	attrs map[string]string
}

type collectedEdge struct {
	src, dst string
	attrs    map[string]string
}

// collector implements gographviz.Interface without attribute-name validation,
// preserving the custom DSL attributes (goal, prompt, condition, ...).
type collector struct {
	name       string
	directed   bool
	graphAttrs map[string]string
	nodes      []collectedNode
	nodeIndex  map[string]int
	edges      []collectedEdge
}

func newCollector() *collector {
	return &collector{graphAttrs: map[string]string{}, nodeIndex: map[string]int{}}
}

func (c *collector) SetStrict(bool) error   { return nil }
func (c *collector) SetDir(d bool) error    { c.directed = d; return nil }
func (c *collector) SetName(n string) error { c.name = n; return nil }
func (c *collector) String() string         { return c.name }

func (c *collector) AddNode(parentGraph, name string, attrs map[string]string) error {
	if idx, ok := c.nodeIndex[name]; ok {
		for k, v := range attrs {
			c.nodes[idx].attrs[k] = v
		}
		return nil
	}
	m := map[string]string{}
	for k, v := range attrs {
		m[k] = v
	}
	c.nodeIndex[name] = len(c.nodes)
	c.nodes = append(c.nodes, collectedNode{name: name, attrs: m})
	return nil
}

func (c *collector) AddEdge(src, dst string, directed bool, attrs map[string]string) error {
	m := map[string]string{}
	for k, v := range attrs {
		m[k] = v
	}
	c.edges = append(c.edges, collectedEdge{src: src, dst: dst, attrs: m})
	return nil
}

func (c *collector) AddPortEdge(src, srcPort, dst, dstPort string, directed bool, attrs map[string]string) error {
	return c.AddEdge(src, dst, directed, attrs)
}

func (c *collector) AddAttr(parentGraph, field, value string) error {
	c.graphAttrs[field] = value
	return nil
}

func (c *collector) AddSubGraph(parentGraph, name string, attrs map[string]string) error {
	return nil
}

func shapeType(shape string) NodeType {
	if t, ok := ShapeToType[shape]; ok {
		return t
	}
	return NodeCodergen
}

func knownNodeType(s string) (NodeType, bool) {
	switch NodeType(s) {
	case NodeStart, NodeExit, NodeCodergen, NodeWaitHuman, NodeConditional,
		NodeParallel, NodeFanIn, NodeTool, NodeManagerLoop:
		return NodeType(s), true
	}
	return "", false
}

func stripQuotes(s string) string { return strings.Trim(s, `"`) }

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func atoiDefault(v string, def int) int {
	if i, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
		return i
	}
	return def
}

// preValidate enforces the spec constraints (spec §2.3) before parsing.
func preValidate(source string) error {
	stripped := reLineComment.ReplaceAllString(reBlockComment.ReplaceAllString(source, ""), "")

	if reStrict.MatchString(stripped) {
		return &aerr.ParseError{Msg: "'strict' modifier is not supported"}
	}

	header := stripped
	if i := strings.Index(stripped, "{"); i >= 0 {
		header = stripped[:i]
	}
	if tokens := strings.Fields(strings.TrimSpace(header)); len(tokens) > 0 && strings.EqualFold(tokens[0], "graph") {
		return &aerr.ParseError{Msg: "Only digraph is supported (undirected graph rejected)"}
	}

	switch len(reDigraph.FindAllString(stripped, -1)) {
	case 0:
		return &aerr.ParseError{Msg: "No digraph found in input"}
	case 1:
		// ok
	default:
		return &aerr.ParseError{Msg: "Only one digraph per file is allowed"}
	}

	if err := rejectUndirectedEdges(stripped); err != nil {
		return err
	}

	return validateNodeIDsAndAttrs(stripped)
}

// rejectUndirectedEdges scans outside quoted strings for the "--" operator.
func rejectUndirectedEdges(s string) error {
	inQuotes := false
	var quoteChar byte
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuotes {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quoteChar {
				inQuotes = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuotes = true
			quoteChar = ch
		} else if ch == '-' && i+1 < len(s) && s[i+1] == '-' {
			if i+2 >= len(s) || s[i+2] != '>' {
				return &aerr.ParseError{Msg: "Undirected edges (--) are not supported, use -> for directed edges"}
			}
		}
	}
	return nil
}

func validateNodeIDsAndAttrs(stripped string) error {
	start := strings.Index(stripped, "{")
	end := strings.LastIndex(stripped, "}")
	if start < 0 || end < 0 {
		return nil
	}
	body := stripped[start+1 : end]

	for _, block := range reAttrBlock.FindAllString(body, -1) {
		content := strings.TrimSpace(block[1 : len(block)-1])
		if content == "" {
			continue
		}
		if err := validateAttrCommas(content); err != nil {
			return err
		}
	}

	bodyNoAttrs := reAttrBlock.ReplaceAllString(body, "")
	bodyNoAttrs = reGraphAttrKV.ReplaceAllString(bodyNoAttrs, "")

	for _, m := range reQuoted.FindAllStringSubmatch(bodyNoAttrs, -1) {
		if !validNodeID.MatchString(m[1]) {
			return &aerr.ParseError{Msg: fmt.Sprintf(
				"Node ID '%s' is not a valid bare identifier. IDs must match [A-Za-z_][A-Za-z0-9_]*. "+
					"Use the 'label' attribute for human-readable names.", m[1])}
		}
	}
	return nil
}

func validateAttrCommas(content string) error {
	matches := reKV.FindAllStringIndex(content, -1)
	if len(matches) <= 1 {
		return nil
	}
	for i := 0; i < len(matches)-1; i++ {
		between := content[matches[i][1]:matches[i+1][0]]
		if !strings.Contains(between, ",") {
			a := strings.TrimSpace(content[matches[i][0]:matches[i][1]])
			b := strings.TrimSpace(content[matches[i+1][0]:matches[i+1][1]])
			return &aerr.ParseError{Msg: fmt.Sprintf(
				"Attributes must be comma-separated. Missing comma between '%s' and '%s'", a, b)}
		}
	}
	return nil
}
