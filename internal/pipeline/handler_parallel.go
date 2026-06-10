package pipeline

import (
	"context"
	"fmt"
	"sync"
)

// ParallelHandler runs the handlers of outgoing target nodes concurrently.
type ParallelHandler struct {
	registry *HandlerRegistry
}

// Execute fans out to branch handlers and joins per the join_policy attribute.
func (h *ParallelHandler) Execute(ctx context.Context, node *Node, pctx *PipelineContext, g *Graph, runDir string) (Outcome, error) {
	outgoing := g.OutgoingEdges(node.ID)
	if len(outgoing) == 0 {
		return Outcome{Status: "success", Notes: "No branches to execute"}, nil
	}
	joinPolicy := node.Attrs["join_policy"]
	if joinPolicy == "" {
		joinPolicy = "wait_all"
	}

	type branchResult struct {
		target  string
		outcome Outcome
	}

	runBranch := func(target string) branchResult {
		targetNode := g.GetNode(target)
		if targetNode == nil || h.registry == nil {
			return branchResult{target, Outcome{Status: "fail", Notes: "No handler"}}
		}
		handler := h.registry.Get(targetNode.Type)
		if handler == nil {
			return branchResult{target, Outcome{Status: "fail", Notes: fmt.Sprintf("No handler for %s", targetNode.Type)}}
		}
		branchCtx := NewPipelineContext(pctx.Snapshot())
		oc, err := handler.Execute(ctx, targetNode, branchCtx, g, runDir)
		if err != nil {
			oc = Outcome{Status: "fail", Notes: err.Error()}
		}
		return branchResult{target, oc}
	}

	resultsCh := make(chan branchResult, len(outgoing))
	var wg sync.WaitGroup
	for _, e := range outgoing {
		wg.Add(1)
		go func(target string) {
			defer wg.Done()
			resultsCh <- runBranch(target)
		}(e.Target)
	}
	wg.Wait()
	close(resultsCh)

	results := map[string]string{}
	for r := range resultsCh {
		results[r.target] = r.outcome.Status
	}

	if joinPolicy == "first_success" {
		for target, status := range results {
			if status == "success" {
				return Outcome{Status: "success", Notes: "First success from " + target}, nil
			}
		}
		return Outcome{Status: "fail", Notes: "No branch succeeded"}, nil
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
