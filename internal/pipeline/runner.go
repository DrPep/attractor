package pipeline

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/nigelpepper/attractor/internal/aerr"
	"github.com/nigelpepper/attractor/internal/agent"
	"github.com/nigelpepper/attractor/internal/llm"
)

// RunResult summarizes a pipeline run.
type RunResult struct {
	Success       bool
	RunID         string
	NodesExecuted []string
	NodeOutcomes  map[string]string
	FinalContext  map[string]any
	Errors        []string
	RunDir        string
}

// RunnerOptions configures a PipelineRunner.
type RunnerOptions struct {
	Client           *llm.Client
	HandlerRegistry  *HandlerRegistry
	Interviewer      Interviewer
	SkillRegistry    *agent.SkillRegistry
	ModelOverride    string
	ProviderOverride string

	OnNodeStart  func(node *Node, index, total int)
	OnNodeEnd    func(nodeID, status string)
	OnEdge       func(src, target, label string)
	OnRetry      func(nodeID string, attempt, maxRetries int, delay float64)
	OnAgentEvent func(agent.Event)
}

// PipelineRunner parses, validates, and executes a pipeline graph.
type PipelineRunner struct {
	registry      *HandlerRegistry
	skillRegistry *agent.SkillRegistry
	opts          RunnerOptions
}

// NewPipelineRunner builds a runner, constructing a default handler registry if
// one is not supplied.
func NewPipelineRunner(opts RunnerOptions) *PipelineRunner {
	registry := opts.HandlerRegistry
	if registry == nil {
		registry = DefaultRegistry(RegistryOptions{
			Client:           opts.Client,
			Interviewer:      opts.Interviewer,
			SkillRegistry:    opts.SkillRegistry,
			ModelOverride:    opts.ModelOverride,
			ProviderOverride: opts.ProviderOverride,
			OnAgentEvent:     opts.OnAgentEvent,
		})
	}
	return &PipelineRunner{registry: registry, skillRegistry: opts.SkillRegistry, opts: opts}
}

// RunParams are the options for a single Run invocation.
type RunParams struct {
	RunDir      string
	Resume      bool
	RunID       string
	RestartFrom string
}

const maxRunnerIterations = 1000

// Run executes a pipeline through all five phases.
func (r *PipelineRunner) Run(ctx context.Context, dotSource string, params RunParams) RunResult {
	runID := params.RunID
	if runID == "" {
		runID = newRunID()
	}
	runDir := params.RunDir
	if runDir == "" {
		runDir = filepath.Join("runs", runID)
	}
	result := RunResult{RunID: runID, RunDir: runDir, NodeOutcomes: map[string]string{}}

	if err := r.run(ctx, dotSource, params, runDir, runID, &result); err != nil {
		log.Printf("Pipeline error: %v", err)
		result.Errors = append(result.Errors, err.Error())
	}
	return result
}

func (r *PipelineRunner) run(ctx context.Context, dotSource string, params RunParams, runDir, runID string, result *RunResult) error {
	// Phase 1: PARSE
	graph, err := ParseDOT(dotSource)
	if err != nil {
		return err
	}

	// Phase 2: VALIDATE
	var knownSkills map[string]bool
	if r.skillRegistry != nil {
		knownSkills = map[string]bool{}
		for _, s := range r.skillRegistry.ListSkills() {
			knownSkills[s.Name] = true
		}
	}
	warnings, err := ValidateOrRaise(graph, knownSkills)
	if err != nil {
		return err
	}
	for _, w := range warnings {
		log.Printf("Validation: %s", w.Message)
	}

	// Phase 3: INITIALIZE
	if err := EnsureRunDir(runDir); err != nil {
		return err
	}
	// Persist the source DOT so the web UI and resumes can recover the graph.
	_ = os.WriteFile(filepath.Join(runDir, "pipeline.dot"), []byte(dotSource), 0o644)
	pctx := NewPipelineContext(nil)
	pctx.Set("graph.goal", graph.Goal)
	pctx.Set("run_id", runID)

	if graph.ModelStylesheet != "" {
		ApplyStylesheet(ParseStylesheet(graph.ModelStylesheet), graph)
	}

	completedNodes := []string{}
	nodeOutcomes := map[string]string{}
	nodeRetries := map[string]int{}
	nodeVisitCounts := map[string]int{}
	startNodeID := ""

	if params.RestartFrom != "" || params.Resume {
		cp, _ := LoadCheckpoint(runDir)
		if cp != nil {
			if params.RestartFrom != "" {
				idx := indexOf(cp.CompletedNodes, params.RestartFrom)
				if idx < 0 {
					return &aerr.PipelineError{Msg: fmt.Sprintf(
						"Cannot restart from '%s': not in completed nodes (%v)", params.RestartFrom, cp.CompletedNodes)}
				}
				pctx.Restore(cp.ContextSnapshot)
				completedNodes = append([]string{}, cp.CompletedNodes[:idx]...)
				kept := toSet(completedNodes)
				for k, v := range cp.NodeRetries {
					if kept[k] {
						nodeRetries[k] = v
					}
				}
				startNodeID = params.RestartFrom
			} else {
				pctx.Restore(cp.ContextSnapshot)
				completedNodes = append([]string{}, cp.CompletedNodes...)
				nodeRetries = cp.NodeRetries
				startNodeID = cp.CurrentNode
			}
			for _, nid := range completedNodes {
				if st, _ := ReadStatus(nid, runDir); st != nil {
					nodeOutcomes[nid] = st.Outcome
				}
			}
		} else if params.RestartFrom != "" {
			return &aerr.PipelineError{Msg: fmt.Sprintf("Cannot restart from '%s': no checkpoint found in %s", params.RestartFrom, runDir)}
		}
	}

	if startNodeID == "" {
		start := graph.StartNode()
		if start == nil {
			return &aerr.PipelineError{Msg: "No start node found"}
		}
		startNodeID = start.ID
	}

	// Phase 4: EXECUTE
	currentNodeID := startNodeID
	iteration := 0
	for iteration < maxRunnerIterations {
		iteration++
		if err := ctx.Err(); err != nil {
			return err
		}
		node := graph.GetNode(currentNodeID)
		if node == nil {
			result.Errors = append(result.Errors, "Node not found: "+currentNodeID)
			break
		}

		pctx.Set("current_node", currentNodeID)

		visitCount := nodeVisitCounts[currentNodeID] + 1
		nodeVisitCounts[currentNodeID] = visitCount
		if node.MaxIterations() > 0 && visitCount > node.MaxIterations() {
			result.Errors = append(result.Errors, fmt.Sprintf("Node '%s' exceeded max_iterations (%d)", node.ID, node.MaxIterations()))
			break
		}

		if r.opts.OnNodeStart != nil {
			r.opts.OnNodeStart(node, len(result.NodesExecuted)+1, len(graph.Nodes))
		}
		log.Printf("Executing node: %s (%s)", node.ID, node.Type)

		outcome := r.executeWithRetry(ctx, node, pctx, graph, runDir, nodeRetries)

		if indexOf(completedNodes, node.ID) < 0 {
			completedNodes = append(completedNodes, node.ID)
		}
		nodeOutcomes[node.ID] = outcome.Status
		result.NodesExecuted = append(result.NodesExecuted, node.ID)
		result.NodeOutcomes[node.ID] = outcome.Status

		if outcome.ContextUpdates != nil {
			pctx.Update(outcome.ContextUpdates)
		}

		_ = WriteStatus(node.ID, NodeStatus{
			Outcome:            outcome.Status,
			PreferredNextLabel: outcome.PreferredLabel,
			SuggestedNextIDs:   outcome.SuggestedNextIDs,
			ContextUpdates:     outcome.ContextUpdates,
			Notes:              outcome.Notes,
		}, runDir)

		_ = SaveCheckpoint(Checkpoint{
			RunID:           runID,
			CurrentNode:     currentNodeID,
			CompletedNodes:  completedNodes,
			NodeRetries:     nodeRetries,
			ContextSnapshot: pctx.Snapshot(),
		}, runDir)

		if r.opts.OnNodeEnd != nil {
			r.opts.OnNodeEnd(currentNodeID, outcome.Status)
		}

		if node.Type == NodeExit {
			gate := CheckGoalGates(graph, nodeOutcomes)
			if !gate.Satisfied {
				if gate.RetryTarget != "" {
					log.Printf("Goal gates unsatisfied: %v. Routing to %s", gate.UnsatisfiedGates, gate.RetryTarget)
					currentNodeID = gate.RetryTarget
					continue
				}
				result.Errors = append(result.Errors, fmt.Sprintf("Goal gates unsatisfied: %v", gate.UnsatisfiedGates))
				break
			}
			result.Success = true
			break
		}

		outgoing := graph.OutgoingEdges(node.ID)
		if len(outgoing) == 0 {
			log.Printf("No outgoing edges from %s, pipeline ends", node.ID)
			result.Success = outcome.Status == "success"
			break
		}

		nextEdge := SelectEdge(outgoing, outcome.Status, outcome.PreferredLabel, outcome.SuggestedNextIDs, pctx)
		if nextEdge == nil {
			result.Errors = append(result.Errors, fmt.Sprintf("No eligible edge from node '%s'", node.ID))
			break
		}
		if r.opts.OnEdge != nil {
			r.opts.OnEdge(node.ID, nextEdge.Target, nextEdge.Label)
		}
		currentNodeID = nextEdge.Target
	}

	if iteration >= maxRunnerIterations {
		result.Errors = append(result.Errors, "Maximum iteration limit reached")
	}

	// Phase 5: FINALIZE
	result.FinalContext = pctx.Snapshot()
	return nil
}

func (r *PipelineRunner) executeWithRetry(ctx context.Context, node *Node, pctx *PipelineContext, graph *Graph, runDir string, nodeRetries map[string]int) Outcome {
	handler := r.registry.Get(node.Type)
	if handler == nil {
		return Outcome{Status: "fail", Notes: "No handler for node type: " + string(node.Type)}
	}

	maxRetries := node.MaxRetries()
	retries := nodeRetries[node.ID]

	for attempt := 0; attempt <= maxRetries; attempt++ {
		outcome, err := handler.Execute(ctx, node, pctx, graph, runDir)
		if err != nil {
			if attempt < maxRetries && IsRetryable(err) {
				retries++
				nodeRetries[node.ID] = retries
				r.backoff(ctx, node.ID, attempt, maxRetries)
				continue
			}
			return Outcome{Status: "fail", Notes: err.Error()}
		}

		if outcome.Status == "retry" && attempt < maxRetries {
			retries++
			nodeRetries[node.ID] = retries
			pctx.Set("internal.retry_count."+node.ID, retries)
			r.backoff(ctx, node.ID, attempt, maxRetries)
			continue
		}

		if outcome.Status == "fail" && attempt < maxRetries {
			retries++
			nodeRetries[node.ID] = retries
			r.backoff(ctx, node.ID, attempt, maxRetries)
			continue
		}

		if outcome.Status == "fail" && node.AllowPartial() {
			outcome.Status = "partial_success"
		}
		return outcome
	}
	return Outcome{Status: "fail", Notes: "Exhausted all retries"}
}

func (r *PipelineRunner) backoff(ctx context.Context, nodeID string, attempt, maxRetries int) {
	delay := ComputeDelay(attempt, RetryStandard)
	if r.opts.OnRetry != nil {
		r.opts.OnRetry(nodeID, attempt+1, maxRetries, delay.Seconds())
	}
	select {
	case <-time.After(delay):
	case <-ctx.Done():
	}
}

func newRunID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// NewRunID generates a fresh random run identifier, matching the IDs the runner
// assigns when none is supplied. Useful for callers that need the ID up front.
func NewRunID() string { return newRunID() }

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}

func toSet(s []string) map[string]bool {
	m := map[string]bool{}
	for _, v := range s {
		m[v] = true
	}
	return m
}
