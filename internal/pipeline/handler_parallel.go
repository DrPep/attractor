package pipeline

import (
	"context"
	"fmt"
)

// ParallelHandler runs the handlers of outgoing target nodes concurrently.
type ParallelHandler struct {
	registry *HandlerRegistry
}

type branchResult struct {
	target  string
	outcome Outcome
}

// Execute fans out to branch handlers and joins per the join_policy attribute.
// "first_success" returns as soon as a branch succeeds and cancels the rest;
// "wait_all" (default) waits for every branch.
func (h *ParallelHandler) Execute(ctx context.Context, node *Node, pctx *PipelineContext, g *Graph, runDir string) (Outcome, error) {
	outgoing := g.OutgoingEdges(node.ID)
	if len(outgoing) == 0 {
		return Outcome{Status: "success", Notes: "No branches to execute"}, nil
	}
	joinPolicy := node.Attrs["join_policy"]
	if joinPolicy == "" {
		joinPolicy = "wait_all"
	}

	branchCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	resultsCh := make(chan branchResult, len(outgoing))
	for _, e := range outgoing {
		go func(target string) {
			resultsCh <- h.runBranch(branchCtx, target, pctx, g, runDir)
		}(e.Target)
	}

	if joinPolicy == "first_success" {
		for i := 0; i < len(outgoing); i++ {
			r := <-resultsCh
			if r.outcome.Status == "success" {
				cancel() // stop the remaining branches
				return Outcome{
					Status:         "success",
					Notes:          "First success from " + r.target,
					ContextUpdates: map[string]any{"parallel.results": map[string]string{r.target: r.outcome.Status}},
				}, nil
			}
		}
		return Outcome{Status: "fail", Notes: "No branch succeeded"}, nil
	}

	// wait_all (default)
	results := map[string]string{}
	for i := 0; i < len(outgoing); i++ {
		r := <-resultsCh
		results[r.target] = r.outcome.Status
	}

	allSuccess := true
	for _, s := range results {
		if s != "success" {
			allSuccess = false
			break
		}
	}
	status := "partial_success"
	if allSuccess {
		status = "success"
	}
	return Outcome{
		Status:         status,
		Notes:          fmt.Sprintf("Parallel: %d branches completed", len(results)),
		ContextUpdates: map[string]any{"parallel.results": results},
	}, nil
}

func (h *ParallelHandler) runBranch(ctx context.Context, target string, pctx *PipelineContext, g *Graph, runDir string) branchResult {
	targetNode := g.GetNode(target)
	if targetNode == nil || h.registry == nil {
		return branchResult{target, Outcome{Status: "fail", Notes: "No handler"}}
	}
	handler := h.registry.Get(targetNode.Type)
	if handler == nil {
		return branchResult{target, Outcome{Status: "fail", Notes: fmt.Sprintf("No handler for %s", targetNode.Type)}}
	}
	// Each branch gets an isolated context snapshot so concurrent branches do
	// not race on the shared PipelineContext.
	branchPctx := NewPipelineContext(pctx.Snapshot())
	oc, err := handler.Execute(ctx, targetNode, branchPctx, g, runDir)
	if err != nil {
		oc = Outcome{Status: "fail", Notes: err.Error()}
	}
	return branchResult{target, oc}
}
