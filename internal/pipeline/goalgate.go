package pipeline

// GoalGateResult reports whether all goal gates are satisfied, and where to
// route otherwise.
type GoalGateResult struct {
	Satisfied        bool
	UnsatisfiedGates []string
	RetryTarget      string
}

// CheckGoalGates verifies every goal-gate node reached success or
// partial_success. completedOutcomes maps node id to outcome status.
func CheckGoalGates(g *Graph, completedOutcomes map[string]string) GoalGateResult {
	acceptable := map[string]bool{"success": true, "partial_success": true}
	var unsatisfied []string
	for _, id := range g.sortedNodeIDs() {
		node := g.Nodes[id]
		if !node.GoalGate() {
			continue
		}
		if !acceptable[completedOutcomes[node.ID]] {
			unsatisfied = append(unsatisfied, node.ID)
		}
	}
	if len(unsatisfied) == 0 {
		return GoalGateResult{Satisfied: true}
	}

	retryTarget := ""
	for _, gateID := range unsatisfied {
		if node := g.GetNode(gateID); node != nil && node.RetryTarget() != "" {
			retryTarget = node.RetryTarget()
			break
		}
	}
	if retryTarget == "" {
		retryTarget = g.RetryTarget
	}
	if retryTarget == "" {
		retryTarget = g.FallbackRetryTarget
	}

	return GoalGateResult{Satisfied: false, UnsatisfiedGates: unsatisfied, RetryTarget: retryTarget}
}
