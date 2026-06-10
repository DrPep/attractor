package pipeline

import "sort"

// SelectEdge picks the next edge using the 5-step priority algorithm:
//  1. keep edges whose condition evaluates true (unconditional edges always kept)
//  2. prefer edges matching preferredLabel
//  3. prefer edges whose target is in suggestedNextIDs
//  4. keep the highest-weight edges
//  5. lexical tiebreak by target id
func SelectEdge(edges []*Edge, outcome, preferredLabel string, suggestedNextIDs []string, ctx *PipelineContext) *Edge {
	if len(edges) == 0 {
		return nil
	}
	if ctx == nil {
		ctx = NewPipelineContext(nil)
	}

	// Step 1: conditions
	var eligible []*Edge
	for _, e := range edges {
		if e.Condition != "" {
			if ok, err := EvaluateCondition(e.Condition, ctx, outcome, preferredLabel); err == nil && ok {
				eligible = append(eligible, e)
			}
		} else {
			eligible = append(eligible, e)
		}
	}
	if len(eligible) == 0 {
		return nil
	}

	// Step 2: preferred_label
	if preferredLabel != "" {
		var matches []*Edge
		for _, e := range eligible {
			if e.Label == preferredLabel {
				matches = append(matches, e)
			}
		}
		if len(matches) > 0 {
			eligible = matches
		}
	}

	// Step 3: suggested_next_ids
	if len(suggestedNextIDs) > 0 && len(eligible) > 1 {
		suggested := map[string]bool{}
		for _, s := range suggestedNextIDs {
			suggested[s] = true
		}
		var matches []*Edge
		for _, e := range eligible {
			if suggested[e.Target] {
				matches = append(matches, e)
			}
		}
		if len(matches) > 0 {
			eligible = matches
		}
	}

	// Step 4: highest weight
	if len(eligible) > 1 {
		maxW := eligible[0].Weight
		for _, e := range eligible {
			if e.Weight > maxW {
				maxW = e.Weight
			}
		}
		var matches []*Edge
		for _, e := range eligible {
			if e.Weight == maxW {
				matches = append(matches, e)
			}
		}
		eligible = matches
	}

	// Step 5: lexical tiebreak
	if len(eligible) > 1 {
		sort.SliceStable(eligible, func(i, j int) bool { return eligible[i].Target < eligible[j].Target })
	}

	return eligible[0]
}
