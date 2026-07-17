package pipeline

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/nigelpepper/attractor/internal/agent"
	"github.com/nigelpepper/attractor/internal/agent/tools"
	"github.com/nigelpepper/attractor/internal/llm"
)

// defaultContextFiles are staged into the agent's working dir if present at cwd.
var defaultContextFiles = []string{"context.md"}

// CodergenHandler expands $goal, runs the agent loop, and writes artifacts.
type CodergenHandler struct {
	client           *llm.Client
	skillRegistry    *agent.SkillRegistry
	modelOverride    string
	providerOverride string
	onAgentEvent     func(agent.Event)
	onSessionStart   func(nodeID string, steer func(message string))
	onSessionEnd     func(nodeID string)
}

// Execute runs the coding agent for a codergen node.
func (h *CodergenHandler) Execute(ctx context.Context, node *Node, pctx *PipelineContext, g *Graph, runDir string) (Outcome, error) {
	// Expand $goal in prompt.
	prompt := node.Prompt()
	goal := pctx.GetString("graph.goal")
	if goal == "" {
		goal = g.Goal
	}
	if strings.Contains(prompt, "$goal") {
		prompt = strings.ReplaceAll(prompt, "$goal", goal)
	}
	if prompt == "" {
		if goal != "" {
			prompt = goal
		} else {
			prompt = "Execute task: " + node.Label
		}
	}

	// Inject prior downstream feedback on re-entry.
	iterKey := "internal.node_iteration." + node.ID
	iteration := toInt(pctx.Get(iterKey, 0))
	if iteration > 0 {
		var feedback []string
		for _, key := range pctx.Keys() {
			if strings.HasSuffix(key, ".response") && key != node.ID+".response" {
				feedback = append(feedback, fmt.Sprintf("[%s]\n%s", key, pctx.GetString(key)))
			}
		}
		if len(feedback) > 0 {
			prompt = prompt + "\n\n--- Feedback from previous iteration ---\n" +
				strings.Join(feedback, "\n\n") + "\n\nPlease address the issues identified above."
		}
	}
	pctx.Set(iterKey, iteration+1)

	_ = WritePrompt(node.ID, prompt, runDir)

	if h.client == nil {
		_ = WriteResponse(node.ID, "(no LLM client configured)", runDir)
		status := "fail"
		if node.AutoStatus() {
			status = "success"
		}
		return Outcome{Status: status, Notes: "No LLM client configured"}, nil
	}

	model := firstNonEmpty(h.modelOverride, node.LLMModel(), llm.DefaultModel)
	provider := firstNonEmpty(h.providerOverride, node.LLMProvider())

	systemPrompt := agent.CodingAgentSystemPrompt
	var toolRegistry *tools.ToolRegistry
	if len(node.Skills()) > 0 && h.skillRegistry != nil {
		composed := h.skillRegistry.Compose(node.Skills())
		if composed.SystemPrompt != "" {
			systemPrompt = systemPrompt + "\n\n" + composed.SystemPrompt
		}
		toolRegistry = h.skillRegistry.BuildToolRegistry(composed)
	}

	providerName := provider
	if providerName == "" {
		providerName = "anthropic"
	}
	profile := agent.NewProviderProfile(providerName, model, systemPrompt, node.ReasoningEffort())
	config := agent.DefaultSessionConfig(model)
	config.Provider = provider
	config.ReasoningEffort = node.ReasoningEffort()

	artifactsDir := filepath.Join(runDir, "artifacts", node.ID)
	if err := os.MkdirAll(artifactsDir, 0o755); err != nil {
		return Outcome{Status: "fail", Notes: err.Error()}, nil
	}
	stageContextFiles(artifactsDir)

	session := agent.NewSession(h.client, agent.SessionOptions{
		Profile:      &profile,
		Environment:  tools.NewLocalEnvironment(artifactsDir),
		Config:       &config,
		ToolRegistry: toolRegistry,
	})

	if h.onAgentEvent != nil {
		nodeID := node.ID
		forward := h.onAgentEvent
		session.OnAllEvents(func(e agent.Event) {
			if e.Data == nil {
				e.Data = map[string]any{}
			}
			if _, ok := e.Data["node_id"]; !ok {
				e.Data["node_id"] = nodeID
			}
			forward(e)
		})
	}

	// Expose the session for inline steering (corrective feedback) while it runs.
	if h.onSessionStart != nil {
		h.onSessionStart(node.ID, session.Steer)
		if h.onSessionEnd != nil {
			defer h.onSessionEnd(node.ID)
		}
	}

	result, err := session.Submit(ctx, prompt)
	if err != nil {
		_ = WriteResponse(node.ID, "Error: "+err.Error(), runDir)
		if node.AutoStatus() {
			return Outcome{Status: "success", Notes: "Auto-status: " + err.Error()}, nil
		}
		return Outcome{Status: "fail", Notes: err.Error()}, nil
	}

	responseText := result.FinalResponse
	_ = WriteResponse(node.ID, responseText, runDir)

	usage := result.Usage
	return Outcome{
		Status:         "success",
		Notes:          fmt.Sprintf("Completed with %d tool calls in %d turns", result.ToolCallsMade, result.TurnsUsed),
		ContextUpdates: map[string]any{node.ID + ".response": responseText},
		Usage:          &usage,
	}, nil
}

// stageContextFiles copies project-root context files into the agent's working
// dir so reads of e.g. ./context.md resolve despite the agent's cwd.
func stageContextFiles(artifactsDir string) {
	cwd, err := os.Getwd()
	if err != nil {
		return
	}
	for _, name := range defaultContextFiles {
		src := filepath.Join(cwd, name)
		info, err := os.Stat(src)
		if err != nil || info.IsDir() {
			continue
		}
		dst := filepath.Join(artifactsDir, name)
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		copyFile(src, dst)
	}
}

func copyFile(src, dst string) {
	in, err := os.Open(src)
	if err != nil {
		return
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return
	}
	defer out.Close()
	_, _ = io.Copy(out, in)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func toInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int64:
		return int(t)
	case float64:
		return int(t)
	default:
		return 0
	}
}
