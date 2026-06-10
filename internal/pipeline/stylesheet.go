package pipeline

import (
	"regexp"
	"sort"
	"strings"
)

// StyleRule is a single CSS-like model stylesheet rule.
type StyleRule struct {
	SelectorType  string // "*", "class", "id"
	SelectorValue string
	Properties    map[string]string
	Specificity   int // 0=universal, 1=class, 2=id
}

var (
	reRuleBlock = regexp.MustCompile(`(?s)([^{]+)\{([^}]*)\}`)
	reProp      = regexp.MustCompile(`([\w_-]+)\s*:\s*"?([^";]+)"?\s*;`)
)

// ParseStylesheet parses CSS-like model stylesheet text into rules.
func ParseStylesheet(css string) []StyleRule {
	var rules []StyleRule
	if strings.TrimSpace(css) == "" {
		return rules
	}
	for _, m := range reRuleBlock.FindAllStringSubmatch(css, -1) {
		selector := strings.TrimSpace(m[1])
		body := strings.TrimSpace(m[2])

		var selType, selValue string
		var specificity int
		switch {
		case selector == "*":
			selType, selValue, specificity = "*", "*", 0
		case strings.HasPrefix(selector, "#"):
			selType, selValue, specificity = "id", selector[1:], 2
		case strings.HasPrefix(selector, "."):
			selType, selValue, specificity = "class", selector[1:], 1
		default:
			selType, selValue, specificity = "class", selector, 1
		}

		props := map[string]string{}
		for _, pm := range reProp.FindAllStringSubmatch(body, -1) {
			props[strings.TrimSpace(pm[1])] = strings.Trim(strings.TrimSpace(pm[2]), `"`)
		}
		if len(props) > 0 {
			rules = append(rules, StyleRule{
				SelectorType: selType, SelectorValue: selValue,
				Properties: props, Specificity: specificity,
			})
		}
	}
	return rules
}

// ApplyStylesheet applies rules to graph nodes. Specificity order is
// * < .class < #id; explicit node attributes are never overridden.
func ApplyStylesheet(rules []StyleRule, g *Graph) {
	sorted := make([]StyleRule, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Specificity < sorted[j].Specificity })

	for _, id := range g.sortedNodeIDs() {
		node := g.Nodes[id]
		resolved := map[string]string{}
		for _, rule := range sorted {
			if styleMatches(rule, node) {
				for k, v := range rule.Properties {
					resolved[k] = v
				}
			}
		}
		for k, v := range resolved {
			if _, exists := node.Attrs[k]; !exists {
				node.Attrs[k] = v
			}
		}
	}
}

func styleMatches(rule StyleRule, node *Node) bool {
	switch rule.SelectorType {
	case "*":
		return true
	case "id":
		return node.ID == rule.SelectorValue
	case "class":
		for _, c := range strings.Fields(node.ClassName()) {
			if c == rule.SelectorValue {
				return true
			}
		}
		return string(node.Type) == rule.SelectorValue
	}
	return false
}
